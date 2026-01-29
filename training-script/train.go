package trainingscript

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/GoogleCloudPlatform/functions-framework-go/functions"
	"go.viam.com/rdk/app"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/utils"

	"net/http"
	"time"
)

const modelName = "cropped-cup-fullness"
const viamDevOrgID = "e76d1b3b-0468-4efd-bb7f-fb1d2b352fcb"
const testDatasetID = "69791f05ecfc7364599781d1"

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
	logger := logging.NewLogger("cloud-func")

	registryItemID := r.URL.Query().Get("registry_item_id")
	version := r.URL.Query().Get("version")
	orgID := r.URL.Query().Get("org_id")

	if registryItemID == "" || version == "" || orgID == "" {
		http.Error(
			w,
			"registry_item_id, version, and org_id are required",
			http.StatusBadRequest,
		)
		return
	}

	viamClient, inferenceClient := connectToViam(ctx, logger)
	defer func() {
		viamClient.Close()
		inferenceClient.Close()
	}()
	logger.Info("authed")

	dataClient := viamClient.DataClient()
	logger.Info("got data")

	ok, err := runTests(
		ctx,
		dataClient,
		inferenceClient,
		testDatasetID,
		orgID,
		version,
		logger,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "tests passed: %v\n", ok)
}

func TrainModelHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logging.NewLogger("cloud-func")

	// get dataset_id and part_id from query parameters
	datasetID := r.URL.Query().Get("dataset_id")
	partID := r.URL.Query().Get("part_id")

	if datasetID == "" {
		http.Error(w, "dataset_id query param not set", http.StatusBadRequest)
		return
	}
	if partID == "" {
		http.Error(w, "part_id query param not set", http.StatusBadRequest)
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
		DatasetID:      datasetID,
		OrganizationID: viamDevOrgID,
		ModelName:      modelName,
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
	trainingJobMetadata, err := pollTrainingStatus(pollCtx, mlClient, trainingID, logger)
	if err != nil {
		if strings.Contains(err.Error(), "canceled") {
			logger.Infof(err.Error())
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	logger.Infof("training complete, got %+v", trainingJobMetadata)

	// test the model
	shouldUpdateConfig, err := runTests(ctx, viamClient.DataClient(), inferenceClient, testDatasetID, viamDevOrgID, newVersionString, logger)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to test trained model %v", err), http.StatusInternalServerError)
		return
	}
	logger.Infof("Tests completed, should update config: %s", shouldUpdateConfig)

	// update config to new model version if model passes the tests
	if shouldUpdateConfig {
		part, _, err := appClient.GetRobotPart(ctx, partID)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to get robot part %v", err), http.StatusInternalServerError)
			return
		}
		cfg := part.RobotConfig
		updateConfig(part.RobotConfig, newVersionString)

		_, err = appClient.UpdateRobotPart(ctx, partID, part.Name, cfg)
		if err != nil {
			http.Error(w, "failed to update robot part", http.StatusInternalServerError)
			return
		}
		logger.Infof("sucessfully updated robot config model version to %s", newVersionString)

	}
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

func updateConfig(oldConfig map[string]interface{}, newVersion string) {
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

func runTests(ctx context.Context, dataClient *app.DataClient, inferenceClient *InferenceClient, testDatasetID string, orgID, versionString string, logger logging.Logger) (bool, error) {
	testImages, err := getTestImages(ctx, dataClient, testDatasetID)
	if err != nil {
		return false, err
	}

	logger.Infof("got %d test images", len(testImages))

	evaluationResult, err := evaluateModel(
		ctx, inferenceClient, versionString, orgID, testImages, logger,
	)
	if err != nil {
		return false, err
	}

	logger.Infof("evaluation complete, %+v", evaluationResult)
	return evaluationResult.Accuracy > 0.95, nil

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
