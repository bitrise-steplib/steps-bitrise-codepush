package main

import (
	"os"

	"github.com/bitrise-io/go-steputils/v2/export"
	"github.com/bitrise-io/go-steputils/v2/stepconf"
	"github.com/bitrise-io/go-utils/v2/command"
	"github.com/bitrise-io/go-utils/v2/env"
	"github.com/bitrise-io/go-utils/v2/log"

	"github.com/bitrise-steplib/steps-bitrise-codepush/step"
)

func main() {
	os.Exit(run())
}

func run() int {
	logger := log.NewLogger()
	codePushStep := createStep(logger)

	config, err := codePushStep.ProcessConfig()
	if err != nil {
		logger.Errorf(err.Error())
		return 1
	}

	result, err := codePushStep.Run(config)
	if err != nil {
		logger.Errorf(err.Error())
		return 1
	}

	if err := codePushStep.ExportOutputs(result); err != nil {
		logger.Errorf(err.Error())
		return 1
	}

	return 0
}

func createStep(logger log.Logger) step.Step {
	envRepository := env.NewRepository()
	inputParser := stepconf.NewInputParser(envRepository)
	cmdFactory := command.NewFactory(envRepository)
	fileManager := export.NewFileManager()
	exporter := export.NewExporter(cmdFactory, fileManager)

	return step.New(inputParser, logger, exporter)
}
