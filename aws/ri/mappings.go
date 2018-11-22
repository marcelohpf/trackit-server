package ri

import (
	"context"
	"time"

	"github.com/trackit/jsonlog"
	"github.com/trackit/trackit-server/es"
)

const IndexPrefixEC2Reserved = "ec2-reserves"
const TypeEC2Reserved = "ec2-reserve"
const TemplateNameEC2Reserved = "ec2-reserves"

func init() {
	ctx, ctxCancel := context.WithTimeout(context.Background(), 10*time.Second)
	res, err := es.Client.IndexPutTemplate(TemplateNameEC2Reserved).BodyString(TemplateReservedInstance).Do(ctx)
	if err != nil {
		jsonlog.DefaultLogger.Error("Failed to put ES index EC2Reserved.", err)
	} else {
		jsonlog.DefaultLogger.Info("Put ES index EC2Reserved.", res)
		ctxCancel()
	}
}

const TemplateReservedInstance = `
{
	"template": "*-ec2-reserves",
	"version": 2,
	"mappings": {
		"ec2-reserve": {
			"properties": {
				"reservedInstancesId": {
					"type": "keyword",
					"norms": false
				},
				"offeringType": {
					"type": "keyword",
					"norms": false
				},
				"endDate": {
					"type": "date"
				},
				"scope": {
					"type": "keyword",
					"norms": false
				},
				"usagePrice": {
					"type": "float",
					"index": false
				},
				"startDate": {
					"type": "date"
				},
				"state": {
					"type": "keyword",
					"norms": false
				},
				"productDescription": {
					"type": "text"
				},
				"currencyCode": {
					"type": "keyword",
					"norms": false
				},
				"duration": {
					"type": "integer",
					"index": false
				},
				"instanceType": {
					"type": "keyword",
					"norms": false
				},
				"instanceCount": {
					"type": "integer",
					"index": false
				},
				"availabilityZone": {
					"type": "keyword",
					"norms": false
				},
				"fixedPrice": {
					"type": "float",
					"index": false
				},
				"offeringClass": {
					"type": "keyword",
					"norms": false
				},
				"family": {
					"type": "keyword",
					"norms": false
				},
				"normalizationFactor": {
					"type": "float",
					"index": false
				},
				"region": {
					"type": "keyword",
					"norms": false
				}
			},
			"_all": {
				"enabled": false
			},
			"numeric_detection": false,
			"date_detection": false
		}
	}
}
`
