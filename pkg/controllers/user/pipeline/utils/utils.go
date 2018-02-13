package utils

import (
	"errors"
	"fmt"
	"github.com/rancher/norman/types"
	"github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/rancher/types/config"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"net/url"
	"strings"
	"time"
)

var CI_ENDPOINT = ""

var PIPELINE_FINISH_LABEL = labels.Set(map[string]string{"pipeline.management.cattle.io/finish": "true"})
var PIPELINE_INPROGRESS_LABEL = labels.Set(map[string]string{"pipeline.management.cattle.io/finish": "false"})

func InitClusterPipeline(cluster *config.UserContext) error {
	clusterPipelines := cluster.Management.Management.ClusterPipelines("")
	clusterPipeline := &v3.ClusterPipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.ClusterName,
			Namespace: cluster.ClusterName,
		},
		Spec: v3.ClusterPipelineSpec{
			ClusterName: cluster.ClusterName,
		},
	}

	if _, err := clusterPipelines.Create(clusterPipeline); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func InitHistory(p *v3.Pipeline, triggerType string) *v3.PipelineExecution {
	history := &v3.PipelineExecution{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getNextHistoryName(p),
			Namespace: p.Namespace,
			Labels:    PIPELINE_INPROGRESS_LABEL,
		},
		Spec: v3.PipelineExecutionSpec{
			ProjectName:  p.Spec.ProjectName,
			PipelineName: p.Name,
			Run:          p.Status.NextRun,
			TriggeredBy:  triggerType,
			Pipeline:     *p,
			//DisplayName: getNextHistoryName(pipeline),
		},
	}
	history.Status.State = StateWaiting
	history.Status.Stages = make([]v3.StageStatus, len(p.Spec.Stages))

	for i := 0; i < len(history.Status.Stages); i++ {
		stage := &history.Status.Stages[i]
		stage.State = StateWaiting
		stepsize := len(p.Spec.Stages[i].Steps)
		stage.Steps = make([]v3.StepStatus, stepsize)
		for j := 0; j < stepsize; j++ {
			step := &stage.Steps[j]
			step.State = StateWaiting
		}
	}
	return history
}

func getNextHistoryName(p *v3.Pipeline) string {
	if p == nil {
		return ""
	}
	return fmt.Sprintf("%s-%d", p.Name, p.Status.NextRun)
}

func IsStageSuccess(stage v3.StageStatus) bool {
	if stage.State == StateSuccess {
		return true
	} else if stage.State == StateFail || stage.State == StateDenied {
		return false
	}
	successSteps := 0
	for _, step := range stage.Steps {
		if step.State == StateSuccess || step.State == StateSkip {
			successSteps++
		}
	}
	return successSteps == len(stage.Steps)
}

func UpdateEndpoint(apiContext *types.APIContext) error {

	reqUrl := apiContext.URLBuilder.Current()
	u, err := url.Parse(reqUrl)
	if err != nil {
		return err
	}
	CI_ENDPOINT = fmt.Sprintf("%s://%s/hooks", u.Scheme, u.Host)
	return nil
}

//FIXME proper way to connect to Jenkins in cluster
func GetJenkinsURL(cluster *config.UserContext) (string, error) {
	nodeLister := cluster.Core.Nodes("").Controller().Lister()
	serviceLister := cluster.Core.Services("").Controller().Lister()
	nodes, err := nodeLister.List("", labels.NewSelector())
	if err != nil {
		return "", err
	}
	if len(nodes) < 1 {
		return "", errors.New("no available nodes")
	}
	if len(nodes[0].Status.Addresses) < 1 {
		return "", errors.New("no available address")
	}
	host := nodes[0].Status.Addresses[0].Address

	svcport := 0
	service, err := serviceLister.Get(PIPELINE_NAMESPACE, "jenkins")
	if err != nil {
		return "", err
	}

	ports := service.Spec.Ports
	for _, port := range ports {
		if port.NodePort != 0 && port.Name == "http" {
			svcport = int(port.NodePort)
			break
		}
	}
	return fmt.Sprintf("http://%s:%d", host, svcport), nil
}

func IsExecutionFinish(execution *v3.PipelineExecution) bool {
	if execution == nil {
		return false
	}
	if execution.Status.State != StateWaiting && execution.Status.State != StateBuilding {
		return true
	}
	return false
}

func RunPipeline(pipelines v3.PipelineInterface, executions v3.PipelineExecutionInterface, logs v3.PipelineExecutionLogInterface, pipeline *v3.Pipeline, triggerType string) (*v3.PipelineExecution, error) {

	//Generate a new pipeline execution
	execution := InitHistory(pipeline, triggerType)
	execution, err := executions.Create(execution)
	if err != nil {
		return nil, err
	}
	//create log entries
	for j, stage := range pipeline.Spec.Stages {
		for k, _ := range stage.Steps {
			log := &v3.PipelineExecutionLog{
				ObjectMeta: metav1.ObjectMeta{
					Name:   fmt.Sprintf("%s-%d-%d", execution.Name, j, k),
					Labels: PIPELINE_INPROGRESS_LABEL,
				},
				Spec: v3.PipelineExecutionLogSpec{
					ProjectName:           pipeline.Spec.ProjectName,
					PipelineExecutionName: execution.Name,
					Stage: j,
					Step:  k,
				},
			}
			if _, err := logs.Create(log); err != nil {
				return nil, err
			}
		}
	}
	pipeline.Status.NextRun++
	pipeline.Status.LastExecutionID = execution.Name
	pipeline.Status.LastStarted = time.Now().Format(time.RFC3339)

	_, err = pipelines.Update(pipeline)
	if err != nil {
		return nil, err
	}
	return execution, nil
}

func SplitImageTag(image string) (string, string, string) {
	registry, repo, tag := "", "", ""
	i := strings.Index(image, "/")
	if i == -1 || (!strings.ContainsAny(image[:i], ".:") && image[:i] != "localhost") {
		registry = DEFAULT_REGISTRY
	} else {
		registry = image[:i]
		image = image[i+1:]
	}
	i = strings.Index(image, ":")
	if i == -1 {
		repo = image
		tag = DEFAULT_TAG
	} else {
		repo = image[:i]
		tag = image[i+1:]
	}
	return registry, repo, tag
}
