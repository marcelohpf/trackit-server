package ri

import (
	"context"
	"database/sql"
	//"github.com/trackit/jsonlog"
	"gopkg.in/olivere/elastic.v5"
	"strings"

	//"github.com/trackit/trackit-server/aws/ri"
	"github.com/trackit/trackit-server/es"
	"github.com/trackit/trackit-server/users"
)

func GetRC2ReportReservedInstances(ctx context.Context, parsedParams RiReportQueryParams, user users.User, tx *sql.Tx) (int, ResponseReservedInstance, error) {
	accountsAndIndexes, returnCode, err := es.GetAccountsAndIndexes(parsedParams.accountList, user, tx, es.IndexPrefixLineItems)
	if err != nil {
		return returnCode, nil, err
	}
	parsedParams.indexList = accountsAndIndexes.Indexes
	returnCode, res, err := makeElasticSearchReportEc2Request(ctx, parsedParams)
	if err != nil {
		return returnCode, nil, err
	}
	returnCode, instances, err := prepareResponseRiResult(ctx, res)
	if err != nil {
		return returnCode, nil, err
	}
	return returnCode, instances, nil

}

func makeElasticSearchReportEc2Request(ctx context.Context, params RiReportQueryParams) (int, *elastic.SearchResult, error) {
	index := strings.Join(params.indexList, ",")
	search := getElasticSearchReportParams(params, es.Client, index)
	return executeElasticSearchQuery(ctx, search, index)
}

func getElasticSearchReportParams(params RiReportQueryParams, client *elastic.Client, index string) *elastic.SearchService {
	query := elastic.NewBoolQuery()
	query = query.Filter(elastic.NewRangeQuery("usageStartDate").From(params.begin))
	query = query.Filter(elastic.NewRangeQuery("usageEndDate").To(params.end))
	query = query.Filter(elastic.NewTermQuery("productCode", "AmazonEC2"))

	search := client.Search().Index(index).Query(query)

	search.Aggregation("usage", elastic.NewTermsAggregation().Field("lineItemType").Size(maxAggregationSize).
		SubAggregation("family", elastic.NewTermsAggregation().Field("instanceTypeFamily").Size(maxAggregationSize).
			SubAggregation("factor", elastic.NewTermsAggregation().Field("normalizationFactor").Size(maxAggregationSize).
				SubAggregation("usageAmount", elastic.NewSumAggregation().Field("normalizedUsageAmount")).
				SubAggregation("usageCost", elastic.NewSumAggregation().Field("unblendedCost")).
				SubAggregation("discountCost", elastic.NewSumAggregation().Field("effectiveCost")))))

	return search
}
