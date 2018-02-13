package pipeline

import (
	"context"
	"github.com/pkg/errors"
	"github.com/rancher/rancher/pkg/controllers/user/pipeline/utils"
	"github.com/rancher/rancher/pkg/ticker"
	"github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/robfig/cron"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/labels"
	"time"
)

const (
	syncInterval = 60 * time.Second
)

type CronSyncer struct {
	pipelineLister        v3.PipelineLister
	pipelines             v3.PipelineInterface
	pipelineExecutions    v3.PipelineExecutionInterface
	pipelienExecutionLogs v3.PipelineExecutionLogInterface
}

func (s *CronSyncer) sync(ctx context.Context, syncInterval time.Duration) {
	for range ticker.Context(ctx, syncInterval) {
		logrus.Debugf("Start sync pipeline cron")
		s.syncCron()
		logrus.Debugf("Sync pipeline cron complete")
	}
}

func (s *CronSyncer) syncCron() {
	pipelines, err := s.pipelineLister.List("", labels.NewSelector())
	if err != nil {
		logrus.Errorf("Error listing pipelines")
		return
	}
	for _, p := range pipelines {
		s.checkCron(p)
	}
}

func (s *CronSyncer) checkCron(pipeline *v3.Pipeline) {
	if pipeline.Spec.TriggerCronExpression == "" {
		return
	}
	if pipeline.Status.NextStart == "" {
		//update nextstart time
		nextStart, err := getNextStartTime(pipeline.Spec.TriggerCronExpression, pipeline.Spec.TriggerCronTimezone, time.Now())
		if err != nil {
			logrus.Errorf("Error getNextStartTime - %v", err)
			return
		}
		pipeline.Status.NextStart = nextStart
		if _, err := s.pipelines.Update(pipeline); err != nil {
			logrus.Errorf("Error update pipeline - %v", err)
		}
		return
	}

	nextStartTime, err := time.ParseInLocation(time.RFC3339, pipeline.Status.NextStart, time.Local)
	if err != nil {
		logrus.Errorf("Error parsing nextStart - %v", err)
		s.resetNextRun(pipeline)
		return
	}
	if nextStartTime.After(time.Now()) {
		//not time up
		return
	} else if nextStartTime.Before(time.Now()) && nextStartTime.Add(syncInterval).After(time.Now()) {
		//trigger run
		nextStart, err := getNextStartTime(pipeline.Spec.TriggerCronExpression, pipeline.Spec.TriggerCronTimezone, time.Now())
		if err != nil {
			logrus.Errorf("Error getNextStartTime - %v", err)
			return
		}
		pipeline.Status.NextStart = nextStart
		if _, err := utils.RunPipeline(s.pipelines, s.pipelineExecutions, s.pipelienExecutionLogs, pipeline, utils.TriggerTypeCron); err != nil {
			logrus.Errorf("Error run pipeline - %v", err)
			return
		}
	} else {
		//stale nextStart
		s.resetNextRun(pipeline)
	}

}

func getNextStartTime(cronExpression string, timezone string, fromTime time.Time) (string, error) {
	//use Local as default
	loc, err := time.LoadLocation(timezone)
	if err != nil || timezone == "" || timezone == "Local" {
		loc = time.Local
		if err != nil {
			logrus.Errorf("Failed to load time zone %v: %+v,use local timezone instead", timezone, err)
		}
	}
	if cronExpression == "* * * * *" {
		return "", errors.New("'* * * * *' for cron is not allowed and ignored")
	}
	schedule, err := cron.ParseStandard(cronExpression)
	if err != nil {
		return "", err
	}

	return schedule.Next(fromTime.In(loc)).Format(time.RFC3339), nil
}

func (s *CronSyncer) resetNextRun(pipeline *v3.Pipeline) {
	pipeline.Status.NextStart = ""
	if _, err := s.pipelines.Update(pipeline); err != nil {
		logrus.Errorf("Error update pipeline - %v", err)
	}
}
