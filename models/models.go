package models

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/thomasmitchell/as2as/ocfas"
	"github.com/thomasmitchell/as2as/pcfas"
)

type Dump struct {
	Spaces []Space `json:"spaces"`
}

type Space struct {
	GUID string `json:"guid"`
	Apps []App  `json:"apps,omitempty"`
}

type App struct {
	GUID                  string                 `json:"guid"`
	Enabled               bool                   `json:"enabled"`
	InstanceLimits        InstanceLimits         `json:"instance_limits"`
	Rules                 []Rule                 `json:"rules,omitempty"`
	ScheduledLimitChanges []ScheduledLimitChange `json:"scheduled_limit_changes,omitempty"`
}

type InstanceLimits struct {
	Min int64 `json:"min"`
	Max int64 `json:"max"`
}

type Rule struct {
	ComparisonMetric string  `json:"comparison_metric,omitempty"`
	Metric           string  `json:"metric,omitempty"`
	QueueName        string  `json:"queue_name,omitempty"`
	RuleType         string  `json:"rule_type"`
	RuleSubType      string  `json:"rule_sub_type,omitempty"`
	ThresholdMin     float64 `json:"threshold_min"`
	ThresholdMax     float64 `json:"threshold_max"`
}

const (
	RuleTypeCPUUtil        = "cpu"
	RuleTypeMemoryUtil     = "memory"
	RuleTypeHTTPThroughput = "http_throughput"
	RuleTypeHTTPLatency    = "http_latency"
	RuleTypeRabbitMQDepth  = "rabbitmq"
)

type ScheduledLimitChange struct {
	Enabled        bool           `json:"enabled"`
	StartTime      TimeOfDay      `json:"start_time"`
	InstanceLimits InstanceLimits `json:"instance_limits"`
	Recurrence     Recurrence     `json:"recurrence"`
}

type TimeOfDay struct {
	Hour   uint8 `json:"hour"`
	Minute uint8 `json:"minute"`
}

type Recurrence uint8

func (r Recurrence) ActiveOn(day time.Weekday) bool {
	return r&(1<<(6-day)) != 0
}

func ConstructApp(
	app pcfas.App,
	rules []pcfas.Rule,
	scheduledLimitChanges []pcfas.ScheduledLimitChange,
) (App, error) {
	ret := App{
		GUID:    app.GUID,
		Enabled: app.Enabled,
		InstanceLimits: InstanceLimits{
			Min: app.InstanceLimits.Min,
			Max: app.InstanceLimits.Max,
		},
	}

	for i := range rules {
		ret.Rules = append(ret.Rules, Rule{
			ComparisonMetric: rules[i].ComparisonMetric,
			Metric:           rules[i].Metric,
			QueueName:        rules[i].QueueName,
			RuleType:         rules[i].RuleType,
			RuleSubType:      rules[i].RuleSubType,
			ThresholdMin:     rules[i].Threshold.Min,
			ThresholdMax:     rules[i].Threshold.Max,
		})
	}

	for i := range scheduledLimitChanges {
		execTime, err := time.Parse(time.RFC3339, scheduledLimitChanges[i].ExecutesAt)
		if err != nil {
			return ret, err
		}

		ret.ScheduledLimitChanges = append(ret.ScheduledLimitChanges, ScheduledLimitChange{
			Enabled: scheduledLimitChanges[i].Enabled,
			StartTime: TimeOfDay{
				Hour:   uint8(execTime.Hour()),
				Minute: uint8(execTime.Minute()),
			},
			InstanceLimits: InstanceLimits{
				Min: scheduledLimitChanges[i].InstanceLimits.Min,
				Max: scheduledLimitChanges[i].InstanceLimits.Max,
			},
			Recurrence: Recurrence(scheduledLimitChanges[i].Recurrence),
		})
	}

	return ret, nil
}

//Returns nil if App is not enabled
func (a App) ToOCFPolicy() (*ocfas.Policy, error) {
	if !a.Enabled {
		return nil, nil
	}

	ret := &ocfas.Policy{
		InstanceMinCount: a.InstanceLimits.Min,
		InstanceMaxCount: a.InstanceLimits.Max,
	}

	for i := range a.Rules {
		var err error
		ret.ScalingRules, err = a.Rules[i].ToOCFScalingRules()
		if err != nil {
			return nil, err
		}
	}

	//TODO: Parse schedules

	return ret, nil
}

var illegalMetricNameRegex = regexp.MustCompile("[^[:alnum:]_]")
var ruleConversionMap = map[string]string{
	RuleTypeCPUUtil:        ocfas.MetricTypeCPUUtil,
	RuleTypeMemoryUtil:     ocfas.MetricTypeMemoryUtil,
	RuleTypeHTTPThroughput: ocfas.MetricTypeThroughput,
	RuleTypeHTTPLatency:    ocfas.MetricTypeResponseTime,
}

func (r *Rule) ToOCFScalingRules() ([]ocfas.ScalingRule, error) {
	var ocfMetricType string
	//RabbitMQ is a special case because it would be a custom metric in OCF
	// However, at this time, RabbitMQ is not exposing queue depth as a
	// metric without https://github.com/starkandwayne/rabbitmq-metrics-emitter-release
	if r.RuleType == RuleTypeRabbitMQDepth {
		//Legal queue names are alphanumeric and underscores. However, queue names may have any UTF8 character. Gross.
		convertedQueueName := strings.ReplaceAll(r.QueueName, "-", "_")
		if illegalMetricNameRegex.MatchString(convertedQueueName) {
			return nil, fmt.Errorf("Illegal metric name generated from RabbitMQ queue name")
		}

		ocfMetricType = fmt.Sprintf("%s_messages_ready", convertedQueueName)
	} else {
		var knownType bool
		ocfMetricType, knownType = ruleConversionMap[r.RuleType]
		if !knownType {
			return nil, fmt.Errorf("Unknown Rule Type `%s'", r.RuleType)
		}
	}

	return []ocfas.ScalingRule{
		{
			MetricType: ocfMetricType,
			Operator:   ocfas.OperatorLessThan,
			Threshold:  int64(r.ThresholdMin), //truncate fractional component
			Adjustment: ocfas.AdjustmentDown,
		},
		{
			MetricType: ocfMetricType,
			Operator:   ocfas.OperatorGreaterThan,
			Threshold:  int64(r.ThresholdMax), //truncate fractional component
			Adjustment: ocfas.AdjustmentUp,
		},
	}, nil
}
