package models

import (
	"time"

	"github.com/thomasmitchell/as2as/pcfas"
)

type Dump struct {
	Spaces []Space `json:"spaces"`
}

type Space struct {
	GUID string `json:"guid"`
	Apps []App  `json:"apps"`
}

type App struct {
	GUID                  string                 `json:"guid"`
	Enabled               bool                   `json:"enabled"`
	InstanceLimits        InstanceLimits         `json:"instance_limits"`
	Rules                 []Rule                 `json:"rules,omitempty"`
	ScheduledLimitChanges []ScheduledLimitChange `json:"scheduled_limit_changes,omitempty"`
}

type InstanceLimits struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

type Rule struct {
	ComparisonMetric string `json:"comparision_metric"`
	Metric           string `json:"metric"`
	QueueName        string `json:"queue_name"`
	RuleType         string `json:"rule_type"`
	RuleSubType      string `json:"rule_sub_type"`
	ThresholdMin     int    `json:"threshold_min"`
	ThresholdMax     int    `json:"threshold_max"`
}

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
