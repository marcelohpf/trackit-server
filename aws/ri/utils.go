package ri

import (
	"context"
	"regexp"
	"time"

	"github.com/trackit/jsonlog"

	taws "github.com/trackit/trackit-server/aws"
	"github.com/trackit/trackit-server/aws/usageReports"
	"github.com/trackit/trackit-server/es"
)

const ReserveInstanceSessionName = "reserved-instances"

type (
	ReservedInstance struct {
		ReservedInstancesId string    `json:"reservedInstancesId"`
		OfferingType        string    `json:"offeringType"`
		EndDate             time.Time `json:"endDate"`
		Scope               string    `json:"scope"`
		UsagePrice          float64   `json:"usagePrice"`
		StartDate           time.Time `json:"startDate"`
		State               string    `json:"state"`
		ProductDescription  string    `json:"productDescription"`
		CurrencyCode        string    `json:"currencyCode"`
		Duration            int64     `json:"duration"`
		InstanceType        string    `json:"instanceType"`
		InstanceCount       int64     `json:"instanceCount"`
		AvailabilityZone    string    `json:"availabilityZone"`
		FixedPrice          float64   `json:"fixedPrice"`
		OfferingClass       string    `json:"offeringClass"`
		Family              string    `json:"family"`
		NormalizationFactor float64   `json:"normalizationFactor"`
		Region              string    `json:"region"`
	}

	ReservedInstanceReport struct {
		Type                string  `json:"type"`
		Family              string  `json:"family"`
		NormalizationFactor float64 `json:"normalizationFactor"`
		NormalizedUsage     float64 `json:"normalizedUsage"`
		Cost                float64 `json:"cost"`
	}
)

// importInstancesToEs receive the reserved instances from EC2 and load in
// ElasticSearch.
// It calls createIndexEs if the index doesn't exist.
func importInstancesToEs(ctx context.Context, aa taws.AwsAccount, instances []ReservedInstance) error {
	logger := jsonlog.LoggerFromContextOrDefault(ctx)
	logger.Info("Updating EC2 reserved instances for AWS account.", map[string]interface{}{
		"awsAccount": aa,
	})
	index := es.IndexNameForUserId(aa.UserId, IndexPrefixEC2Reserved)
	bp, err := utils.GetBulkProcessor(ctx)
	if err != nil {
		logger.Error("Failed to get bulk processor.", err.Error())
		return err
	}
	for _, instance := range instances {
		id := instance.ReservedInstancesId
		bp = utils.AddDocToBulkProcessor(bp, instance, TypeEC2Reserved, index, id)
	}
	bp.Flush()
	err = bp.Close()
	if err != nil {
		logger.Error("Fail to put EC2 instances in ES", err.Error())
		return err
	}
	logger.Info("EC2 instances put in ES", nil)
	return nil
}

// normalizeInstanceType normalize the computation factor of a
// instance as https://docs.aws.amazon.com/awsaccountbilling/latest/aboutv2/enhanced-lineitem-columns.html
func familyNormalizeFactor(instanceType string) (string, float64) {
	normalizationFactor := map[string]float64{
		"nano":     0.25,
		"micro":    0.5,
		"small":    1,
		"medium":   2,
		"large":    4,
		"xlarge":   8,
		"2xlarge":  16,
		"4xlarge":  32,
		"8xlarge":  64,
		"9xlarge":  72,
		"10xlarge": 80,
		"12xlarge": 96,
		"16xlarge": 128,
		"18xlarge": 144,
		"24xlarge": 192,
		"32xlarge": 256,
	}
	regex, _ := regexp.Compile("(.+)\\.([0-9]*x?[a-z]+$)")
	matchs := regex.FindStringSubmatch(instanceType)
	if len(matchs) == 3 {
		return matchs[1], normalizationFactor[matchs[2]]
	} else {
		return "unknown", 0
	}
}
