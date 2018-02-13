package pipeline

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/rancher/norman/api/access"
	"github.com/rancher/norman/httperror"
	"github.com/rancher/norman/types"
	"github.com/rancher/rancher/pkg/controllers/user/pipeline/utils"
	"github.com/rancher/types/client/management/v3"
	"github.com/rancher/types/config"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"strconv"
	"strings"
)

type ExecutionHandler struct {
	Management config.ManagementContext
}

func (h ExecutionHandler) ExecutionFormatter(apiContext *types.APIContext, resource *types.RawResource) {
	resource.AddAction(apiContext, "rerun")
	resource.AddAction(apiContext, "stop")
	resource.Links["log"] = apiContext.URLBuilder.Link("log", resource)

}

func (h ExecutionHandler) LinkHandler(apiContext *types.APIContext, next types.RequestHandler) error {
	logrus.Debugf("enter link - %v", apiContext.Link)
	if apiContext.Link == "log" {
		return h.log(apiContext)
	}

	return httperror.NewAPIError(httperror.NotFound, "Link not found")

}

func (h *ExecutionHandler) ActionHandler(actionName string, action *types.Action, apiContext *types.APIContext) error {
	logrus.Debugf("do execution action:%s", actionName)

	switch actionName {
	case "rerun":
		return h.rerun(apiContext)
	case "deactivate":
		return h.stop(apiContext)
	}

	return httperror.NewAPIError(httperror.InvalidAction, "unsupported action")
}

func (h *ExecutionHandler) rerun(apiContext *types.APIContext) error {
	return nil
}

func (h *ExecutionHandler) stop(apiContext *types.APIContext) error {
	return nil
}

func (h *ExecutionHandler) log(apiContext *types.APIContext) error {
	stage := apiContext.Request.URL.Query().Get("stage")
	step := apiContext.Request.URL.Query().Get("step")
	if stage == "" || step == "" {
		return errors.New("Step index for log is not provided")
	}

	logId := fmt.Sprintf("%s-%s-%s", apiContext.ID, stage, step)
	data := map[string]interface{}{}
	if err := access.ByID(apiContext, apiContext.Version, client.PipelineExecutionLogType, logId, &data); err != nil {
		return err
	}

	apiContext.WriteResponse(http.StatusOK, data)
	return nil
}
