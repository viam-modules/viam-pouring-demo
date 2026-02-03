package trainingscript

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/GoogleCloudPlatform/functions-framework-go/functions"
	"go.viam.com/rdk/app"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/utils"

	"net/http"
	"time"
)

type TrainingRequest struct {
	DatasetID         string   `json:"dataset_id"`
	PartID            string   `json:"part_id"`
	ModelName         string   `json:"model_name"`
	OrganizationID    string   `json:"org_id"`
	TestDatasetID     string   `json:"test_dataset_id"`
	Labels            []string `json:"labels"`
	AccuracyThreshold float64  `json:"accuracy_threshold,omitempty"`
}

func init() {
	functions.HTTP("TrainModelHTTP", TrainModelHTTP)
}

func TrainModelHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/test":
		// using for debug purposes
		TestModelHandler(w, r)
	case "/train":
		TrainModelHandler(w, r)
	default:
		http.NotFound(w, r)
	}
}

func TestModelHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logging.NewLogger("tests-func")

	testDatasetID := r.URL.Query().Get("test_dataset_id")
	modelName := r.URL.Query().Get("model_name")
	registryItemID := r.URL.Query().Get("registry_item_id")
	version := r.URL.Query().Get("version")
	orgID := r.URL.Query().Get("org_id")
	accuracyThreshold := r.URL.Query().Get("accuracy_threshold")

	if accuracyThreshold == "" {
		accuracyThreshold = "0.95"
	}
	accuracyThresholdFloat, err := strconv.ParseFloat(accuracyThreshold, 64)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid accuracy threshold: %v", err), http.StatusBadRequest)
		return
	}

	if registryItemID == "" || version == "" || orgID == "" || testDatasetID == "" || modelName == "" {
		http.Error(
			w,
			"registry_item_id, version, test_dataset_id, and org_id are required",
			http.StatusBadRequest,
		)
		return
	}

	viamClient, inferenceClient := connectToViam(ctx, logger)
	defer func() {
		viamClient.Close()
		inferenceClient.Close()
	}()

	dataClient := viamClient.DataClient()

	_, err = runTests(
		ctx,
		dataClient,
		inferenceClient,
		testDatasetID,
		orgID,
		modelName,
		version,
		accuracyThresholdFloat,
		logger,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func TrainModelHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logging.NewLogger("cloud-func")

	// get request params
	req, err := decodeTrainingRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// setup
	viamClient, inferenceClient := connectToViam(ctx, logger)
	defer func() {
		viamClient.Close()
		inferenceClient.Close()
	}()

	mlClient := viamClient.MLTrainingClient()
	appClient := viamClient.AppClient()

	// submit training job
	newVersionString := fmt.Sprintf("%s-script", time.Now().UTC().Format("2006-01-02T15-04-05"))
	trainingID, err := mlClient.SubmitTrainingJob(ctx, app.SubmitTrainingJobArgs{
		DatasetID:      req.DatasetID,
		OrganizationID: req.OrganizationID,
		ModelName:      req.ModelName,
		ModelVersion:   newVersionString,
	}, app.ModelTypeSingleLabelClassification, []string{"full", "not-full"})
	if err != nil {
		http.Error(w, fmt.Sprintf("submit training job failed: %v", err), http.StatusInternalServerError)
		return
	}
	logger.Infof("Training job submitted with ID %s", trainingID)

	// poll and wait for training completion
	pollCtx, cancel := context.WithTimeout(ctx, 50*time.Minute)
	defer cancel()
	_, err = pollTrainingStatus(pollCtx, mlClient, trainingID, logger)
	if err != nil {
		if strings.Contains(err.Error(), "canceled") {
			logger.Infof(err.Error())
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	logger.Infof("training complete")

	// test the model
	shouldUpdateConfig, err := runTests(
		ctx, viamClient.DataClient(), inferenceClient, req.TestDatasetID, req.OrganizationID, req.ModelName,
		newVersionString, req.AccuracyThreshold, logger,
	)
	if err != nil {
		http.Error(w, fmt.Sprintf("an error occurred while testing the new model: %v", err), http.StatusInternalServerError)
		return
	}

	logger.Infof("should update config: %v", shouldUpdateConfig)

	// update config to new model version if model passes the tests
	if shouldUpdateConfig {
		part, _, err := appClient.GetRobotPart(ctx, req.PartID)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to get robot part %v", err), http.StatusInternalServerError)
			return
		}
		cfg := part.RobotConfig
		updateConfig(part.RobotConfig, req.ModelName, newVersionString)

		_, err = appClient.UpdateRobotPart(ctx, req.PartID, part.Name, cfg)
		if err != nil {
			http.Error(w, "failed to update robot part", http.StatusInternalServerError)
			return
		}
		logger.Infof("sucessfully updated robot config model version to %s", newVersionString)

	}
}

func decodeTrainingRequest(r *http.Request) (*TrainingRequest, error) {
	var req TrainingRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, fmt.Errorf("invalid json body: %w", err)
	}

	if req.DatasetID == "" ||
		req.PartID == "" ||
		req.ModelName == "" ||
		req.OrganizationID == "" ||
		req.TestDatasetID == "" ||
		len(req.Labels) < 2 {
		return nil, fmt.Errorf("missing required fields")
	}

	if req.AccuracyThreshold == 0 {
		req.AccuracyThreshold = 0.95
	}

	return &req, nil
}

func pollTrainingStatus(
	ctx context.Context,
	mlClient *app.MLTrainingClient,
	trainingID string,
	logger logging.Logger,
) (*app.TrainingJobMetadata, error) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("timed out")
			return nil, ctx.Err()

		case <-ticker.C:
			md, err := mlClient.GetTrainingJob(ctx, trainingID)
			if err != nil {
				return nil, err
			}

			switch md.Status {
			case app.TrainingStatusCompleted:
				return md, nil
			case app.TrainingStatusFailed:
				return nil, fmt.Errorf("training failed")
			case app.TrainingStatusCanceled, app.TrainingStatusCanceling:
				return nil, fmt.Errorf("training canceled")
			}
		}
	}
}

func updateConfig(oldConfig map[string]interface{}, modelName, newVersion string) {
	pkgs, ok := oldConfig["packages"].([]interface{})
	if !ok {
		panic("packages is not an array")
	}

	for _, p := range pkgs {
		pkgMap, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		if pkgMap["name"] == modelName {
			pkgMap["version"] = newVersion
		}
	}
}

func runTests(
	ctx context.Context, dataClient *app.DataClient, inferenceClient *InferenceClient,
	testDatasetID, orgID, modelName, versionString string, accuracyThreshold float64, logger logging.Logger,
) (bool, error) {
	testImages, err := getTestImages(ctx, dataClient, testDatasetID)
	if err != nil {
		return false, err
	}

	logger.Infof("got %d test images", len(testImages))

	evaluationResult, err := evaluateModel(ctx, inferenceClient, modelName, versionString, orgID, testImages)
	if err != nil {
		return false, err
	}

	testsPassed := evaluationResult.Accuracy > accuracyThreshold

	logger.Infof("tests passed: %v", testsPassed)

	datasetName, err := createTestResultsDataset(ctx, dataClient, orgID, modelName, versionString, evaluationResult.FailedImages)
	if err != nil {
		return testsPassed, err
	}

	logger.Infof("test failures dataset created: %s", datasetName)

	logEvaluationSummary(logger, evaluationResult)

	return testsPassed, nil
}

func connectToViam(ctx context.Context, logger logging.Logger) (*app.ViamClient, *InferenceClient) {
	viamClient, err := app.CreateViamClientFromEnvVars(ctx, nil, logger)
	if err != nil {
		logger.Fatal(err)
	}

	apiKey := os.Getenv(utils.APIKeyEnvVar)
	apiKeyID := os.Getenv(utils.APIKeyIDEnvVar)
	mlInferenceClient, err := NewInferenceClient(ctx, apiKey, apiKeyID)
	if err != nil {
		logger.Fatal(err)
	}

	return viamClient, mlInferenceClient
}
