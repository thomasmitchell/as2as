package models

import (
	"fmt"
	"regexp"
	"sort"
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
	GUID                  string                `json:"guid"`
	Enabled               bool                  `json:"enabled"`
	InstanceLimits        InstanceLimits        `json:"instance_limits"`
	Rules                 []Rule                `json:"rules,omitempty"`
	ScheduledLimitChanges ScheduledLimitChanges `json:"scheduled_limit_changes,omitempty"`
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

type ScheduledLimitChanges []ScheduledLimitChange

type TimeOfDay struct {
	Hour   uint8 `json:"hour"`
	Minute uint8 `json:"minute"`
}

func (t TimeOfDay) String() string {
	return fmt.Sprintf("%02d:%02d", t.Hour, t.Minute)
}

func (t TimeOfDay) LessThan(t2 TimeOfDay) bool {
	if t.Hour != t2.Hour {
		return t.Hour < t2.Hour
	}

	return t.Minute <= t2.Minute
}

func (t TimeOfDay) SubOneMinute() TimeOfDay {
	if t.Minute == 0 {
		t.Minute = 59

		if t.Hour == 0 {
			t.Hour = 23
		} else {
			t.Hour--
		}
	} else {
		t.Minute--
	}

	return t
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

	minCount, maxCount := a.InstanceLimits.Min, a.InstanceLimits.Max
	if minCount <= 0 {
		minCount = 1
	}

	if maxCount <= 0 {
		maxCount = 1
	}

	ret := &ocfas.Policy{
		InstanceMinCount: minCount,
		InstanceMaxCount: maxCount,
	}

	for i := range a.Rules {
		var err error
		ret.ScalingRules, err = a.Rules[i].ToOCFScalingRules()
		if err != nil {
			return nil, err
		}
	}
	recurringScheds := a.ScheduledLimitChanges.ToOCFRecurringSchedules()
	if len(recurringScheds) > 0 {
		ret.Schedules = &ocfas.Schedules{
			Timezone:          "Etc/UTC",
			RecurringSchedule: recurringScheds,
		}
	}

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

//Returns nil if Schedule not enabled
func (s ScheduledLimitChanges) ToOCFRecurringSchedules() []ocfas.RecurringSchedule {
	splitScheds := daySchedules{}
	for _, sched := range s {
		if !sched.Enabled {
			continue
		}

		splitScheds = append(splitScheds, sched.toDaySchedules()...)
	}

	if len(splitScheds) == 0 {
		return nil
	}

	if len(splitScheds) == 1 {
		initial := (splitScheds[0].InstanceLimits.Min + splitScheds[0].InstanceLimits.Max) / 2
		return []ocfas.RecurringSchedule{
			{
				StartTime:               TimeOfDay{0, 0}.String(),
				EndTime:                 TimeOfDay{23, 59}.String(),
				DaysOfWeek:              ocfas.DaysOfWeek{1, 2, 3, 4, 5, 6, 7},
				InstanceMinCount:        splitScheds[0].InstanceLimits.Min,
				InstanceMaxCount:        splitScheds[0].InstanceLimits.Max,
				InitialMinInstanceCount: &initial,
			},
		}
	}

	//This turns the starting point based schedules of PCF to the OCF representations of the periods of
	// time between the starting points.
	verboseRet := splitScheds.ToOCF()

	return condenseOCFRecurringSchedules(verboseRet)
}

//Monday is 1, Tuesday is 2....
func weekdayToOCF(day time.Weekday) int8 {
	return int8((day+6)%7) + 1
}

type daySchedule struct {
	Weekday        time.Weekday
	StartTime      TimeOfDay
	InstanceLimits InstanceLimits
}

type daySchedules []daySchedule

//First thing on Sunday to last thing on Saturday
func (d daySchedules) Sort() {
	sort.Slice(d, func(i, j int) bool {
		if d[i].Weekday != d[j].Weekday {
			return d[i].Weekday < d[j].Weekday
		}

		return d[i].StartTime.LessThan(d[j].StartTime)
	})
}

func (d daySchedules) ToOCF() []ocfas.RecurringSchedule {
	d.Sort()

	periods := make([]ocfas.RecurringSchedule, 0, len(d)-1)
	var toAppend *ocfas.RecurringSchedule

	for idx, sched := range d {
		if toAppend != nil {
			split := false
			for i := d[idx-1].Weekday + 1; i < sched.Weekday; i++ {
				//Fill in days which did not have schedule starting points
				periods = append(periods, ocfas.RecurringSchedule{
					StartTime:               TimeOfDay{0, 0}.String(),
					EndTime:                 TimeOfDay{23, 59}.String(),
					DaysOfWeek:              ocfas.DaysOfWeek{weekdayToOCF(i)},
					InstanceMinCount:        toAppend.InstanceMinCount,
					InstanceMaxCount:        toAppend.InstanceMaxCount,
					InitialMinInstanceCount: toAppend.InitialMinInstanceCount,
				})
				split = true
			}

			endTime := sched.StartTime.SubOneMinute()

			if split || endTime.LessThan(d[idx-1].StartTime) {
				//Need to split into two rules if would cross day boundary
				toAppend.EndTime = TimeOfDay{23, 59}.String()
				periods = append(periods, *toAppend)

				toAppend = &ocfas.RecurringSchedule{
					StartTime:               TimeOfDay{0, 0}.String(),
					DaysOfWeek:              ocfas.DaysOfWeek{weekdayToOCF(sched.Weekday)},
					InstanceMinCount:        toAppend.InstanceMinCount,
					InstanceMaxCount:        toAppend.InstanceMaxCount,
					InitialMinInstanceCount: toAppend.InitialMinInstanceCount,
				}
			}

			toAppend.EndTime = endTime.String()
			periods = append(periods, *toAppend)
		}

		initial := (sched.InstanceLimits.Min + sched.InstanceLimits.Max) / 2
		toAppend = &ocfas.RecurringSchedule{
			StartTime:               sched.StartTime.String(),
			DaysOfWeek:              ocfas.DaysOfWeek{weekdayToOCF(sched.Weekday)},
			InstanceMinCount:        sched.InstanceLimits.Min,
			InstanceMaxCount:        sched.InstanceLimits.Max,
			InitialMinInstanceCount: &initial,
		}
	}

	split := false
	for i := d[len(d)-1].Weekday + 1; i != d[0].Weekday; i = (i + 1) % (time.Saturday + 1) {
		periods = append(periods, ocfas.RecurringSchedule{
			StartTime:               TimeOfDay{0, 0}.String(),
			EndTime:                 TimeOfDay{23, 59}.String(),
			DaysOfWeek:              ocfas.DaysOfWeek{weekdayToOCF(i)},
			InstanceMinCount:        toAppend.InstanceMinCount,
			InstanceMaxCount:        toAppend.InstanceMaxCount,
			InitialMinInstanceCount: toAppend.InitialMinInstanceCount,
		})

		split = true
	}

	endTime := d[0].StartTime.SubOneMinute()

	if split || endTime.LessThan(d[len(d)-1].StartTime) {
		//Need to split into two rules if would cross day boundary
		toAppend.EndTime = TimeOfDay{23, 59}.String()
		periods = append(periods, *toAppend)

		toAppend = &ocfas.RecurringSchedule{
			StartTime:               TimeOfDay{0, 0}.String(),
			DaysOfWeek:              ocfas.DaysOfWeek{weekdayToOCF(d[0].Weekday)},
			InstanceMinCount:        toAppend.InstanceMinCount,
			InstanceMaxCount:        toAppend.InstanceMaxCount,
			InitialMinInstanceCount: toAppend.InitialMinInstanceCount,
		}
	}

	toAppend.EndTime = endTime.String()
	periods = append(periods, *toAppend)
	return periods
}

var daysOfWeek = [7]time.Weekday{
	time.Sunday,
	time.Monday,
	time.Tuesday,
	time.Wednesday,
	time.Thursday,
	time.Friday,
	time.Saturday,
}

func (s *ScheduledLimitChange) toDaySchedules() daySchedules {
	ret := daySchedules{}
	for _, weekday := range daysOfWeek {
		if s.Recurrence.ActiveOn(weekday) {
			ret = append(ret, daySchedule{
				Weekday:        weekday,
				StartTime:      s.StartTime,
				InstanceLimits: s.InstanceLimits,
			})
		}
	}

	return ret
}

func condenseOCFRecurringSchedules(in []ocfas.RecurringSchedule) []ocfas.RecurringSchedule {
	ret := []ocfas.RecurringSchedule{}

	inClone := make([]ocfas.RecurringSchedule, len(in))
	for i := range in {
		inClone[i] = in[i]
	}

	for i := 0; i < len(inClone); i++ {
		toAppend := inClone[i]

		for j := i + 1; j < len(inClone); j++ {
			if inClone[j].StartTime == toAppend.StartTime &&
				inClone[j].EndTime == toAppend.EndTime &&
				inClone[j].InstanceMinCount == toAppend.InstanceMinCount &&
				inClone[j].InstanceMaxCount == toAppend.InstanceMaxCount {

				toAppend.DaysOfWeek = append(toAppend.DaysOfWeek, inClone[j].DaysOfWeek...)

				inClone[j], inClone[len(inClone)-1] = inClone[len(inClone)-1], inClone[j]
				inClone = inClone[:len(inClone)-1]
				j--
			}
		}

		sort.Slice(toAppend.DaysOfWeek, func(i, j int) bool { return toAppend.DaysOfWeek[i] < toAppend.DaysOfWeek[j] })
		ret = append(ret, toAppend)
	}

	return ret
}

type Converted struct {
	Spaces []ConvertedSpace `json:"spaces"`
}

type ConvertedSpace struct {
	GUID string                 `json:"guid"`
	Apps []ConvertedPolicyToApp `json:"apps,omitempty"`
}

type ConvertedPolicyToApp struct {
	GUID   string        `json:"guid"`
	Policy *ocfas.Policy `json:"policy,omitempty"`
}
