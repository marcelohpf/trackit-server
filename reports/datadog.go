package reports

import (
	//"context"

	"github.com/DataDog/datadog-go/statsd"
	//"github.com/trackit/jsonlog"
	//"github.com/trackit/trackit-server/config"
)

const DATADOG_TRACKIT = "trackit."

func submitDatadogMetrics( /*ctx context.Context, */ gi *GeneralInformation) {

	//logger := jsonlog.LoggerFromContextOrDefault(ctx)
	c, err := statsd.New("datadog-agent.datadog.svc.cluster.local.:8125")

	if err != nil {
		//logger.Error("Failed to create datadog client", err)
	} else {

		c.Namespace = DATADOG_TRACKIT

		err = c.Gauge("rds.instances", float64(gi.TotalInstancesRds), nil, 1)
		err = c.Gauge("rds.low_used.instances", float64(gi.LowUsedInstancesRds), nil, 1)
		err = c.Gauge("ec2.low_used.instances", float64(gi.LowUsedInstancesEc2), nil, 1)
		err = c.Gauge("ec2.reserved_instances.expiration", float64(gi.ReservesWillExpire), nil, 1)
	}
	//err = c.Gauge("ec2.unreserved.expiration", gi.UnreservedEc2Instances, nil, 1)
}
