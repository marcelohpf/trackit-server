package reports

import (
	"golang.org/x/text/message"
)

const MONTHDAYS = 30.4365

func formatGb(value float64) (string, float64) {
	formats := []string{"B", "KB", "MB", "GB", "TB", "PT", "EB", "ZB"}

	byteValue := value * 1024 * 1024 * 1024
	i := 0
	for byteValue/1024 >= 1 {
		byteValue /= 1024
		i++
	}

	return formats[i], byteValue
}

func fToS(float float64) string {
	printer := message.NewPrinter(message.MatchLanguage("en"))
	return printer.Sprintf("%-.2f", float)
}
