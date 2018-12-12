package reports

import (
	//"context"

	"github.com/DataDog/datadog-go/statsd"
	//"github.com/trackit/jsonlog"
	//"github.com/trackit/trackit-server/config"
)

const DATADOG_TRACKIT = "trackit."
const DATADOG_URL = "datadog-agent.datadog.svc.cluster.local.:8125"

func submitDatadogMetrics( /*ctx context.Context, */ gi *GeneralInformation) {

	//logger := jsonlog.LoggerFromContextOrDefault(ctx)
	c, err := statsd.New(DATADOG_URL)

	defer c.Close()

	if err != nil {
		//logger.Error("Failed to create datadog client", err)
	} else {

		c.Namespace = DATADOG_TRACKIT

		err = c.Gauge("rds.instances", float64(gi.TotalInstancesRds), nil, 1)
		err = c.Gauge("rds.low_used.instances", float64(gi.LowUsedInstancesRds), nil, 1)
		err = c.Gauge("ec2.low_used.instances", float64(gi.LowUsedInstancesEc2), nil, 1)
		err = c.Gauge("ec2.reserved_instances.expiration", float64(gi.Reservations.ReservesWillExpire), nil, 1)
	}
	//err = c.Gauge("ec2.unreserved.expiration", gi.UnreservedEc2Instances, nil, 1)
}

func submitUnreservedEc2(suggestions []UnreservedSuggestion) {
	c, err := statsd.New(DATADOG_URL)

	defer c.Close()

	if err != nil {
		// logger
	} else {
		c.Namespace = DATADOG_TRACKIT
		for _, unreserved := range suggestions {
			err := c.Gauge("ec2.unreserved.instances", float64(unreserved.Machines), []string{"instance-type:" + unreserved.InstanceType}, 1)
			if err != nil {
				// warning
			}

		}
	}
}
