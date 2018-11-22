//   Copyright 2017 MSolution.IO
//
//   Licensed under the Apache License, Version 2.0 (the "License");
//   you may not use this file except in compliance with the License.
//   You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
//   Unless required by applicable law or agreed to in writing, software
//   distributed under the License is distributed on an "AS IS" BASIS,
//   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//   See the License for the specific language governing permissions and
//   limitations under the License.

package s3

import (
	"context"
	"time"

	"github.com/trackit/jsonlog"

	"github.com/trackit/trackit-server/es"
)

const TypeLineItem = "lineitem"
const IndexPrefixLineItem = "lineitems"
const TemplateNameLineItem = "lineitems"

// put the ElasticSearch index for *-lineitems indices at startup.
func init() {
	ctx, ctxCancel := context.WithTimeout(context.Background(), 10*time.Second)
	res, err := es.Client.IndexPutTemplate(TemplateNameLineItem).BodyString(TemplateLineItem).Do(ctx)
	if err != nil {
		jsonlog.DefaultLogger.Error("Failed to put ES index lineitems.", err)
	} else {
		jsonlog.DefaultLogger.Info("Put ES index lineitems.", res)
		ctxCancel()
	}
}

const TemplateLineItem = `
{
	"template": "*-lineitems",
	"version": 8,
	"mappings": {
		"lineitem": {
			"properties": {
          "availabilityZone": {
            "type": "keyword",
						"norms": false
          },
          "billRepositoryId": {
            "type": "integer"
          },
          "currencyCode": {
            "type": "keyword",
						"norms": false
          },
          "effectiveCost": {
            "type": "float",
            "index": false
          },
          "instanceTypeFamily": {
            "type": "keyword",
						"norms": false
          },
          "invoiceId": {
            "type": "keyword",
						"norms": false
          },
          "lineItemId": {
            "type": "keyword",
						"norms": false
          },
          "lineItemType": {
            "type": "keyword",
						"norms": false
          },
          "normalizationFactor": {
            "type": "float",
            "index": false
          },
          "normalizedUnitsPerReservation": {
            "type": "float",
            "index": false
          },
          "normalizedUsageAmount": {
            "type": "float",
            "index": false
          },
          "numberOfReservations": {
            "type": "float",
            "index": false
          },
					"billType": {
						"type": "keyword",
						"index": false
					},
          "operation": {
            "type": "keyword",
						"norms": false
          },
          "productCode": {
            "type": "keyword",
						"norms": false
          },
          "region": {
            "type": "keyword",
						"norms": false
          },
          "resourceId": {
            "type": "keyword",
						"norms": false
          },
          "serviceCode": {
            "type": "keyword",
						"norms": false
          },
          "tags": {
            "type": "nested",
            "properties": {
              "key": {
                "type": "keyword",
								"norms": false
              },
              "tag": {
                "type": "keyword",
								"norms": false
              }
            }
          },
          "taxType": {
            "type": "keyword",
						"norms": false
          },
          "term": {
            "type": "keyword",
						"norms": false
          },
          "timeInterval": {
            "type": "keyword",
						"norms": false
          },
          "totalReservedNormalizedUnits": {
            "type": "float",
            "index": false
          },
          "totalReservedUnits": {
            "type": "float",
            "index": false
          },
          "unblendedCost": {
            "type": "float",
            "index": false
          },
          "unusedNormalizedUnitQuantity": {
            "type": "float",
            "index": false
          },
          "unusedQuantity": {
            "type": "float",
            "index": false
          },
          "usageAccountId": {
            "type": "keyword"
          },
          "usageAmount": {
            "type": "float",
            "index": false
          },
          "usageEndDate": {
            "type": "date"
          },
          "usageStartDate": {
            "type": "date"
          },
          "usageType": {
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
