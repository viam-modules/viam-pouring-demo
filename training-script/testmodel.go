package trainingscript

import (
	"context"
	"fmt"
	"os"
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
}

func getTestImages(ctx context.Context, dataClient *app.DataClient, datasetID string) ([]*app.BinaryData, error) {
	fmt.Print(os.Getenv("VIAM_API_KEY"))
	request := &app.DataByFilterOptions{
		Filter: &app.Filter{
			OrganizationIDs: []string{"e76d1b3b-0468-4efd-bb7f-fb1d2b352fcb"},
			LocationIDs:     []string{"kssbd6djf3"},
			DatasetID:       "69791f05ecfc7364599781d1",
			Interval:        app.CaptureInterval{Start: time.Now().AddDate(0, 0, -7), End: time.Now()},
		},
		Limit: 200,
	}

	response, err := dataClient.BinaryDataByFilter(ctx, false, request)
	if err != nil {
		fmt.Printf("error %s", err)
		return nil, err
	}
	return response.BinaryData, nil
}

func evaluateModel(
	ctx context.Context, inferenceClient *InferenceClient, registryItemVersion, organizationID string, testImages []*app.BinaryData, logger logging.Logger,
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
		updateMetrics(result, expectedClass, predictedClass)
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

func updateMetrics(result *EvaluationResult, expected, predicted string) {
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
