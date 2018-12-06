//   Copyright 2017 MSolution.IO
//
//   Licensed under the Apache License, Version 2.0 (the "License");
//   you may not use this file except in compliance with the License.
//   You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
//   distributed under the License is distributed on an "AS IS" BASIS,
//   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//   See the License for the specific language governing permissions and
//   limitations under the License.

package product

import (
	"context"
	"errors"
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/pricing"

	"github.com/trackit/jsonlog"

	taws "github.com/trackit/trackit-server/aws"
	"github.com/trackit/trackit-server/config"
)

type (
	EC2Product struct {
		Family              string
		Normalization       float64
		InstanceType        string
		InstancePricing     float64
		HourlyPricing       float64
		PurchaseOption      string
		PriceCurrency       string
		LeaseContractLength string
		OfferingClass       string
	}
	EC2ProductsPrice map[string]float64
)

const HOURSYEAR = 8765.81256
const HOURSMONTH = 730.48438
const EC2ProductSessionName = "GetEC2Products"

func GetProductsEC2HourlyPrice(ctx context.Context) (EC2ProductsPrice, error) {
	logger := jsonlog.LoggerFromContextOrDefault(ctx)
	creds, err := taws.GetTemporaryCredentials()

	if err != nil {
		logger.Error("Error when getting temporary credentials", err.Error())
		return nil, err
	}

	products, err := fetchProducts(ctx, creds)
	if err != nil {
		logger.Error("Error when fetching products", err.Error())
		return nil, err
	}

	productsPrice := EC2ProductsPrice{}
	for _, product := range products {
		productsPrice[product.InstanceType] = product.HourlyPricing
	}
	return productsPrice, nil
}

func fetchProducts(ctx context.Context, creds *credentials.Credentials) ([]EC2Product, error) {
	logger := jsonlog.LoggerFromContextOrDefault(ctx)

	sess, _ := session.NewSession(&aws.Config{
		Credentials: creds,
		Region:      aws.String(config.AwsRegion),
	})

	svc := pricing.New(sess)
	input := &pricing.GetProductsInput{
		Filters: []*pricing.Filter{
			{
				Type:  aws.String("TERM_MATCH"),
				Field: aws.String("PurchaseOption"),
				Value: aws.String("All Upfront"),
			},
			{
				Type:  aws.String("TERM_MATCH"),
				Field: aws.String("ProductFamily"),
				Value: aws.String("Compute Instance"),
			},
			{
				Type:  aws.String("TERM_MATCH"),
				Field: aws.String("location"),
				Value: aws.String("US East (N. Virginia)"),
			},
			{
				Type:  aws.String("TERM_MATCH"),
				Field: aws.String("operatingSystem"),
				Value: aws.String("Linux"),
			},
			{
				Type:  aws.String("TERM_MATCH"),
				Field: aws.String("preInstalledSw"),
				Value: aws.String("NA"),
			},
			{
				Type:  aws.String("TERM_MATCH"),
				Field: aws.String("tenancy"),
				Value: aws.String("Shared"),
			},
		},
		FormatVersion: aws.String("aws_v1"),
		MaxResults:    aws.Int64(100),
		ServiceCode:   aws.String("AmazonEC2"),
		NextToken:     aws.String(""), // first fetch
	}

	var products []EC2Product

	for { // fetch while has products
		result, err := svc.GetProducts(input)

		if err != nil {
			return nil, err
		}

		for _, item := range result.PriceList { // process the batch products
			ec2product, err := processItemList(item)
			if err != nil {
				logger.Warning("Fail to process a item in list of Products.", err.Error())
			} else {
				products = append(products, ec2product)
			}
		}

		if result.NextToken != nil {
			input.SetNextToken(*result.NextToken)
		} else {
			break
		}

	}
	return products, nil
}

func processItemList(item aws.JSONValue) (EC2Product, error) {
	terms := item["terms"].(map[string]interface{})
	reserveds := terms["Reserved"].(map[string]interface{})

	for _, ireserved := range reserveds {
		//fmt.Println("reserved")
		reserved := ireserved.(map[string]interface{})
		termAttributes := reserved["termAttributes"].(map[string]interface{})

		purchaseOption := termAttributes["PurchaseOption"].(string)
		offeringClass := termAttributes["OfferingClass"].(string)
		leaseContractLength := termAttributes["LeaseContractLength"].(string)

		if purchaseOption == "All Upfront" && offeringClass == "standard" && leaseContractLength == "1yr" {

			priceDimensions := reserved["priceDimensions"].(map[string]interface{})

			for _, ipriceDimension := range priceDimensions {

				priceDimension := ipriceDimension.(map[string]interface{})

				if priceDimension["unit"].(string) == "Quantity" { // search for only up front fee
					pricePerUnit := priceDimension["pricePerUnit"].(map[string]interface{})
					product := item["product"].(map[string]interface{})
					attributes := product["attributes"].(map[string]interface{})
					strPricing := pricePerUnit["USD"].(string)

					pricing, err := strconv.ParseFloat(strPricing, 64)

					if err != nil {
						return EC2Product{}, err
					}

					ec2Product := EC2Product{
						Family:              "",
						Normalization:       0.0,
						InstanceType:        attributes["instanceType"].(string),
						InstancePricing:     pricing,
						HourlyPricing:       pricing / HOURSYEAR,
						PurchaseOption:      purchaseOption,
						PriceCurrency:       "USD",
						LeaseContractLength: leaseContractLength,
						OfferingClass:       offeringClass,
					}
					return ec2Product, nil
				}
			}
		}
	}
	return EC2Product{}, errors.New("Not found a Full Upfront, 1 year, with USD values.")
}
