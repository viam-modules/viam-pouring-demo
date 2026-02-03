package trainingscript

import (
	"context"
	"fmt"
	"strings"
	"time"

	datapb "go.viam.com/api/app/data/v1"
	mlinferencepb "go.viam.com/api/app/mlinference/v1"
	"go.viam.com/rdk/app"
	"go.viam.com/rdk/logging"
)

type ClassMetrics struct {
	Total    int     `json:"total"`
	Correct  int     `json:"correct"`
	Accuracy float64 `json:"accuracy"`
}

type EvaluationResult struct {
	TotalSamples       int                       `json:"total_samples"`
	CorrectPredictions int                       `json:"correct_predictions"`
	Accuracy           float64                   `json:"accuracy"`
	PerClassMetrics    map[string]ClassMetrics   `json:"per_class_metrics"`
	ConfusionMatrix    map[string]map[string]int `json:"confusion_matrix"`
	FailedImages       []string                  `json:"failed_images"`
}

func getTestImages(ctx context.Context, dataClient *app.DataClient, datasetID string) ([]*app.BinaryData, error) {
	request := &app.DataByFilterOptions{
		Filter: &app.Filter{
			DatasetID: datasetID,
		},
	}

	response, err := dataClient.BinaryDataByFilter(ctx, false, request)
	if err != nil {
		fmt.Printf("error %s", err)
		return nil, err
	}

	return response.BinaryData, nil
}

func evaluateModel(
	ctx context.Context, inferenceClient *InferenceClient, modelName, registryItemVersion, organizationID string, testImages []*app.BinaryData,
) (*EvaluationResult, error) {
	result := &EvaluationResult{
		PerClassMetrics: make(map[string]ClassMetrics),
		ConfusionMatrix: make(map[string]map[string]int),
	}

	for _, testImage := range testImages {
		if len(testImage.Metadata.CaptureMetadata.Tags) == 0 {
			continue
		}
		expectedClass := testImage.Metadata.CaptureMetadata.Tags[0]
		response, err := inferenceClient.client.GetInference(ctx, &mlinferencepb.GetInferenceRequest{
			RegistryItemId:      fmt.Sprintf("%s:%s", organizationID, modelName),
			RegistryItemVersion: registryItemVersion,
			BinaryDataId:        testImage.Metadata.BinaryDataID,
			OrganizationId:      organizationID,
		})
		if err != nil {
			return nil, err
		}

		predictedClass := extractBestLabel(response.Annotations.Classifications, 0.6)
		updateMetrics(result, testImage.Metadata.BinaryDataID, expectedClass, predictedClass)
	}

	return result, nil
}

func extractBestLabel(classifications interface{}, threshold float64) string {
	var bestLabel string
	bestConfidence := float64(0)
	switch c := classifications.(type) {

	case []*app.Classification:
	case []*datapb.Classification:
		for _, cls := range c {
			if cls == nil {
				continue
			}
			if *cls.Confidence > bestConfidence {
				bestConfidence = *cls.Confidence
				bestLabel = cls.Label
			}
		}
	}

	if bestConfidence < threshold {
		return ""
	}

	return bestLabel
}

func updateMetrics(result *EvaluationResult, binaryDataID, expected, predicted string) {
	result.TotalSamples++

	// Initialize class metrics if needed
	if _, exists := result.PerClassMetrics[expected]; !exists {
		result.PerClassMetrics[expected] = ClassMetrics{}
	}
	if _, exists := result.ConfusionMatrix[expected]; !exists {
		result.ConfusionMatrix[expected] = make(map[string]int)
	}

	// Update metrics
	classMetrics := result.PerClassMetrics[expected]
	classMetrics.Total++

	if expected == predicted {
		result.CorrectPredictions++
		classMetrics.Correct++
	} else {
		result.FailedImages = append(result.FailedImages, binaryDataID)
	}

	// Calculate accuracies
	if classMetrics.Total > 0 {
		classMetrics.Accuracy = float64(classMetrics.Correct) / float64(classMetrics.Total)
	}
	if result.TotalSamples > 0 {
		result.Accuracy = float64(result.CorrectPredictions) / float64(result.TotalSamples)
	}

	result.PerClassMetrics[expected] = classMetrics
	result.ConfusionMatrix[expected][predicted]++
}

func logEvaluationSummary(logger logging.Logger, result *EvaluationResult) {
	// Overall summary
	logger.Infof("Evaluation: Total=%d, Correct=%d, Accuracy=%.2f%%",
		result.TotalSamples, result.CorrectPredictions, result.Accuracy*100)

	// Per-class accuracies (compact)
	perClass := make([]string, 0, len(result.PerClassMetrics))
	for class, metrics := range result.PerClassMetrics {
		perClass = append(perClass, fmt.Sprintf("%s:%.2f%%", class, metrics.Accuracy*100))
	}
	logger.Infof("Per-class accuracy: %s", strings.Join(perClass, ", "))

	// Confusion matrix as a compact table
	classes := make([]string, 0, len(result.ConfusionMatrix))
	for class := range result.ConfusionMatrix {
		classes = append(classes, class)
	}

	// header with fixed width
	header := fmt.Sprintf("%-12s", "Actual\\Pred")
	for _, class := range classes {
		header += fmt.Sprintf("%10s", class)
	}
	logger.Infof(header)

	// rows
	for actual, row := range result.ConfusionMatrix {
		line := fmt.Sprintf("%-12s", actual)
		for _, predicted := range classes {
			line += fmt.Sprintf("%10d", row[predicted])
		}
		logger.Infof(line)
	}
}

func createTestResultsDataset(
	ctx context.Context, dataClient *app.DataClient, organizationID, modelName, version string, failedImages []string,
) (string, error) {
	testRunTime := time.Now().Format("20060102T150405")
	cleanedModelVersion := strings.ReplaceAll(version, "-", "")
	datasetName := fmt.Sprintf("test-failures_%s_%s_run%s", modelName, cleanedModelVersion, testRunTime)

	datasetID, err := dataClient.CreateDataset(ctx, datasetName, organizationID)
	if err != nil {
		return "", err
	}

	err = dataClient.AddBinaryDataToDatasetByIDs(ctx, failedImages, datasetID)
	if err != nil {
		return "", err
	}

	return datasetName, nil
}
