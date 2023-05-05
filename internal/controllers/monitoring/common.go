package monitoring

type MonitoringStatus string

const (
	MonitoringStatusPending MonitoringStatus = "Pending"
	MonitoringStatusReady   MonitoringStatus = "Ready"
)
