package ocfas

type Policy struct {
	InstanceMinCount int64         `json:"instance_min_count"`
	InstanceMaxCount int64         `json:"instance_max_count"`
	ScalingRules     []ScalingRule `json:"scaling_rules,omitempty"`
	Schedules        Schedules     `json:"schedules,omitempty"`
}

const (
	MetricTypeMemoryUsed   = "memoryused"
	MetricTypeMemoryUtil   = "memoryutil"
	MetricTypeCPUUtil      = "cpu"
	MetricTypeResponseTime = "responsetime"
	MetricTypeThroughput   = "throughput"
	MetricTypeCustom       = "custom"
)

const (
	OperatorLessThan             string = "<"
	OperatorLessThanOrEqualTo    string = "<="
	OperatorGreaterThan          string = ">"
	OperatorGreaterThanOrEqualTo string = ">="
)

const (
	AdjustmentDown string = "-1"
	AdjustmentUp   string = "+1"
)

type ScalingRule struct {
	MetricType         string `json:"metric_type"`
	Operator           string `json:"operator"`
	Threshold          int64  `json:"threshold"`
	Adjustment         string `json:"adjustment"`
	CooldownSecs       int64  `json:"cool_down_secs,omitempty"`
	BreachDurationSecs int64  `json:"breach_duration_secs,omitempty"`
}

const (
	TimezoneUTC = "Etc/UTC"
)

type Schedules struct {
	Timezone          string              `json:"timezone,omitempty"`
	RecurringSchedule []RecurringSchedule `json:"recurring_schedule,omitempty"`
	SpecificDate      []SpecificDate      `json:"specific_date,omitempty"`
}

type RecurringSchedule struct {
	StartTime        string     `json:"start_time"`
	EndTime          string     `json:"end_time"`
	DaysOfWeek       DaysOfWeek `json:"days_of_week"`
	InstanceMinCount int64      `json:"instance_min_count"`
	InstanceMaxCount int64      `json:"instance_max_count"`
	//TODO: Make into pointer?
	InitialMinInstanceCount *int64 `json:"initial_min_instance_count,omitempty"`
}

type DaysOfWeek []int8

type SpecificDate struct {
	StartDateTime           string `json:"start_date_time"`
	EndDateTime             string `json:"end_date_time"`
	InstanceMinCount        int64  `json:"instance_min_count"`
	InstanceMaxCount        int64  `json:"instance_max_count"`
	InitialMinInstanceCount *int64 `json:"initial_min_instance_count,omitempty"`
}
