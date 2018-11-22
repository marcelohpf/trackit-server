//   Copyright 2018 MSolution.IO
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

package reports

import (
	"context"
	"fmt"
	"io"
	"path"
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/tealeg/xlsx"

	"github.com/trackit/jsonlog"
	taws "github.com/trackit/trackit-server/aws"
	"github.com/trackit/trackit-server/awsSession"
	"github.com/trackit/trackit-server/config"
)

type spreadsheet struct {
	account taws.AwsAccount
	date    string
	file    *xlsx.File
}

type sheet struct {
	name string
	data [][]cell
}

type cell struct {
	value string
	width int
	style *xlsx.Style
}

func newCell(value string, dimensions ...int) cell {
	width := 1
	if len(dimensions) > 0 {
		width = dimensions[0]
	}
	return cell{
		value: value,
		width: width,
		style: xlsx.NewStyle(),
	}
}

func convertToSheet(raw sheet) (sheet xlsx.Sheet) {
	var horizontalPadding int
	sheet = xlsx.Sheet{Name: raw.name}
	for _, rawRow := range raw.data {
		horizontalPadding = 0
		row := sheet.AddRow()
		for _, rawCell := range rawRow {
			for horizontalPadding > 0 {
				row.AddCell()
				horizontalPadding--
			}
			newCell := row.AddCell()
			newCell.Value = rawCell.value
			if rawCell.width > 1 {
				rawCell.width--
				newCell.HMerge = rawCell.width
				horizontalPadding = rawCell.width
			}
			newCell.SetStyle(rawCell.style)
		}
	}
	return
}

func generateSpreadsheet(ctx context.Context, aa taws.AwsAccount, date string, sheets []sheet) (*spreadsheet, map[string]error) {
	logger := jsonlog.LoggerFromContextOrDefault(ctx)
	logger.Info("Generating spreadsheet", aa)

	file := xlsx.NewFile()
	errors := make(map[string]error)
	for _, rawSheet := range sheets {
		logger.Info("Adding sheet", rawSheet.name)
		sheet := convertToSheet(rawSheet)
		_, err := file.AppendSheet(sheet, rawSheet.name)
		if err != nil {
			logger.Error("Error while adding sheet "+rawSheet.name, err.Error())
			errors[rawSheet.name] = err
		}
	}
	return &spreadsheet{account: aa, date: date, file: file}, errors
}

func saveSpreadsheetLocally(ctx context.Context, file *spreadsheet) (err error) {
	logger := jsonlog.LoggerFromContextOrDefault(ctx)

	filename := fmt.Sprintf("/reports/%s.xlsx", file.date)

	err = file.file.Save(filename)
	if err != nil {
		logger.Error("Error while saving file", err)
	}
	return
}

func saveSpreadsheet(ctx context.Context, file *spreadsheet) (err error) {
	logger := jsonlog.LoggerFromContextOrDefault(ctx)

	filename := fmt.Sprintf("%s.xlsx", file.date)
	reportPath := path.Join(strconv.Itoa(file.account.Id), "generated-report", filename)

	logger.Info("Uploading spreadsheet ", reportPath)

	reader, writer := io.Pipe()

	go func() {
		defer writer.Close()
		err := file.file.Write(writer)
		if err != nil {
			logger.Error("Error while saving report "+reportPath, err.Error())
		}
	}()

	uploader := s3manager.NewUploader(awsSession.Session)
	result, err := uploader.Upload(&s3manager.UploadInput{
		Body:   reader,
		Bucket: aws.String(config.ReportsBucket),
		Key:    aws.String(reportPath),
	})
	if err != nil {
		logger.Error("Failed to upload report "+reportPath, err.Error())
	} else {
		logger.Info("Successfully uploaded to", result.Location)
	}
	return
}
