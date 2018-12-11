package tags

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"gopkg.in/olivere/elastic.v5"

	"github.com/trackit/jsonlog"
	terrors "github.com/trackit/trackit-server/errors"
	"github.com/trackit/trackit-server/es"
)

// .*[a-z]+[0-9]+[a-z]*.[0-9]*[a-z]+.*|.*Storage.*
// .*[a-z]+[0-9]+[a-z]*.[0-9]*[a-z]+.*
type (
	ResourceTags struct {
		Owner       string  `json:"owner"`
		Application string  `json:"application"`
		Ec2Cost     float64 `json:"ec2Cost"`
		RdsCost     float64 `json:"RdsCost"`
	}

	Ec2TagsValuesQueryParams struct {
		AccountList []string  `json:"awsAccounts"`
		IndexList   []string  `json:"indexes"`
		DateBegin   time.Time `json:"begin"`
		DateEnd     time.Time `json:"end"`
	}

	esProductsTagsValueResult struct {
		Buckets []struct {
			TagGroup    string `json:"key"`
			ProductCode struct {
				Buckets []struct {
					Product string `json:"key"`
					Cost    struct {
						Value float64 `json:"value"`
					} `json:"cost"`
				} `json:"buckets"`
			} `json:"productCode"`
		} `json:"buckets"`
	}

	ProductsTagsResponse []ResourceTags
)

func getEc2GroupedTagsWithParsedParams(ctx context.Context, params Ec2TagsValuesQueryParams) (int, ProductsTagsResponse, error) {
	logger := jsonlog.LoggerFromContextOrDefault(ctx)

	res, returnCode, err := makeElasticSearchRequestEC2GroupTag(ctx, params, es.Client)

	var response ProductsTagsResponse
	if err != nil {
		if returnCode == http.StatusOK {
			return returnCode, response, nil
		}
		return returnCode, response, terrors.GetErrorMessage(ctx, err)
	}

	var typedDocument esProductsTagsValueResult
	err = json.Unmarshal(*res.Aggregations["data"], &typedDocument)
	if err != nil {
		logger.Error("Error while unmarshaling", err)
		return http.StatusInternalServerError, response, terrors.GetErrorMessage(ctx, err)
	}

	for _, tag_group := range typedDocument.Buckets {
		tag_group.TagGroup = strings.TrimLeft(tag_group.TagGroup, "[")
		tag_group.TagGroup = strings.TrimRight(tag_group.TagGroup, "]")
		tags := strings.Split(tag_group.TagGroup, ",")
		if len(tags) != 2 {
			return http.StatusInternalServerError, response, errors.New("The ES hasn't returned a tagroup with two values for only EC2 Instances Grouped by tags.")
		}
		// The elasticsearch query sort by Tag Key, so first is Application then Owner
		tagsValue := ResourceTags{
			Application: tags[0],
			Owner:       tags[1],
		}

		for _, products := range tag_group.ProductCode.Buckets {
			if products.Product == "AmazonEC2" {
				tagsValue.Ec2Cost = products.Cost.Value
			} else { // is rds
				tagsValue.RdsCost = products.Cost.Value
			}
		}
		response = append(response, tagsValue)
	}
	return http.StatusOK, response, nil
}

func makeElasticSearchRequestEC2GroupTag(ctx context.Context, params Ec2TagsValuesQueryParams, client *elastic.Client) (*elastic.SearchResult, int, error) {
	//logger := jsonlog.LoggerFromContextOrDefault(ctx)
	query := getTagsQuery(params)
	index := strings.Join(params.IndexList, ",")

	tags := map[string]interface{}{"tags": []string{"Application", "Owner"}}

	aggregation := elastic.NewTermsAggregation().
		Script(elastic.NewScriptInline("params._source.tags.sort( (b,c) -> b['key'].compareTo(c['key']));"+
			"List a = new ArrayList(); for(item in params._source.tags) {"+
			"if (params.tags.contains(item.key)) { "+
			"a.add(item.tag); "+
			"}} return a.toString().toLowerCase();").
			Params(tags)).
		Size(maxAggregationSize).
		SubAggregation("productCode", elastic.NewTermsAggregation().Field("productCode").Size(maxAggregationSize).
			SubAggregation("cost", elastic.NewSumAggregation().Field("unblendedCost")))

	// Custom query aggregation
	search := client.Search().Index(index).Size(0).Query(query)
	search.Aggregation("data", aggregation)
	return runQueryElasticSearch(ctx, index, search)
}

func getTagsQuery(params Ec2TagsValuesQueryParams) *elastic.BoolQuery {
	query := elastic.NewBoolQuery()

	if len(params.AccountList) > 0 {
		query = query.Filter(createQueryAccountFilter(params.AccountList))
	}

	query = query.Filter(elastic.NewRangeQuery("usageStartDate").From(params.DateBegin).To(params.DateEnd))
	query = query.Filter(elastic.NewTermQuery("lineItemType", "Usage"))
	// this regex search for Storage usage of RDS and Instances usage for RDS and EC2
	query = query.Filter(elastic.NewRegexpQuery("usageType", ".*[a-z]+[0-9]+[a-z]*.[0-9]*[a-z]+.*|.*Storage.*"))
	query = query.Filter(elastic.NewTermsQuery("productCode", "AmazonEC2", "AmazonRDS"))
	return query
}

/*
{
  "aggs": {
    "group": {
      "terms": {
        "script": {
          "source": "params._source.tags.sort( (b,c) -> b['key'].compareTo(c['key'])); List a = new ArrayList(); for (item in params._source.tags) { if(['Application', 'Owner'].contains(item.key)) { a.add(item.tag); } } return a.toString().toLowerCase();"
        },
        "size": 2000
      },
      "aggs": {
        "value": {
          "sum": {
            "field": "unblendedCost"
          }
        },
        "resourcesId": {
          "terms": {
            "field": "resourceId"
          }
        }
      }
    }
  }
}
*/
