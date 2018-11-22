package ri

import (
	// "fmt"
	"context"
	"sync"
	//"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"

	"github.com/trackit/jsonlog"

	taws "github.com/trackit/trackit-server/aws"
	"github.com/trackit/trackit-server/config"

	"github.com/trackit/trackit-server/aws/usageReports"
)

// fetchReservedInstances fetch EC2 reserved instances from aws api for a specific region
// The role should have permission read in aws reserved instances
func fetchReservedInstances(ctx context.Context, creds *credentials.Credentials, region string, instanceChan chan ReservedInstance, wg *sync.WaitGroup) error {
	logger := jsonlog.LoggerFromContextOrDefault(ctx)
	defer wg.Done()
	sess := session.Must(session.NewSession(&aws.Config{
		Credentials: creds,
		Region:      aws.String(region),
	}))
	svc := ec2.New(sess)
	desc := ec2.DescribeReservedInstancesInput{}
	instances, err := svc.DescribeReservedInstances(&desc)
	if err != nil {
		logger.Error("Error when describing reserved instances", err.Error())
		return err
	}

	for _, instance := range instances.ReservedInstances {
		awsType := aws.StringValue(instance.InstanceType)
		family, normalizationFactor := familyNormalizeFactor(awsType)
		instanceChan <- ReservedInstance{
			ReservedInstancesId: aws.StringValue(instance.ReservedInstancesId),
			OfferingType:        aws.StringValue(instance.OfferingType),
			EndDate:             aws.TimeValue(instance.End),
			Scope:               aws.StringValue(instance.Scope),
			UsagePrice:          aws.Float64Value(instance.UsagePrice),
			StartDate:           aws.TimeValue(instance.Start),
			State:               aws.StringValue(instance.State),
			ProductDescription:  aws.StringValue(instance.ProductDescription),
			CurrencyCode:        aws.StringValue(instance.CurrencyCode),
			Duration:            aws.Int64Value(instance.Duration),
			InstanceCount:       aws.Int64Value(instance.InstanceCount),
			AvailabilityZone:    aws.StringValue(instance.AvailabilityZone),
			FixedPrice:          aws.Float64Value(instance.FixedPrice),
			OfferingClass:       aws.StringValue(instance.OfferingClass),
			InstanceType:        awsType,
			Family:              family,
			NormalizationFactor: normalizationFactor,
			Region:              region,
		}
	}
	return nil
}

// FetchReservedInstances fetch reserved instances in AWS EC2 api of an AwsAccount and load it in ElasticSearch
func FetchReservedInstances(ctx context.Context, awsAccount taws.AwsAccount) error {
	logger := jsonlog.LoggerFromContextOrDefault(ctx)
	creds, err := taws.GetTemporaryCredentials(awsAccount, ReserveInstanceSessionName)
	if err != nil {
		logger.Error("Error when getting temporary credentials", err.Error())
		return err
	}

	defaultSession := session.Must(session.NewSession(&aws.Config{
		Credentials: creds,
		Region:      aws.String(config.AwsRegion),
	}))

	// now := time.Now().UTC()
	//account, err := utils.GetAccountId(ctx, defaultSession)
	// if err != nil {
	// 	logger.Error("Error when getting account id", err.Error())
	// 	return err
	// }
	regions, err := utils.FetchRegionsList(ctx, defaultSession)
	if err != nil {
		logger.Error("Error when fetching regions list", err.Error())
		return err
	}

	wg := sync.WaitGroup{}

	riChan := make(chan ReservedInstance)
	for _, region := range regions {
		wg.Add(1)
		go fetchReservedInstances(ctx, creds, region, riChan, &wg)
	}

	// Throw a go routine to close the channel when fetch instances end and avoid dead blocks
	go func(ri chan ReservedInstance, wg *sync.WaitGroup) {
		wg.Wait()
		close(ri)
	}(riChan, &wg)

	var instances []ReservedInstance
	for instance := range riChan {
		instances = append(instances, instance)
	}

	return importInstancesToEs(ctx, awsAccount, instances)
}
