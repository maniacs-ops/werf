package deploy

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/flant/logboek"
	"github.com/flant/werf/pkg/config"
	"github.com/flant/werf/pkg/deploy/helm"
	"github.com/flant/werf/pkg/tag_strategy"
	"github.com/ghodss/yaml"
)

type DeployOptions struct {
	Values               []string
	SecretValues         []string
	Set                  []string
	SetString            []string
	Timeout              time.Duration
	Env                  string
	UserExtraAnnotations map[string]string
	UserExtraLabels      map[string]string
	IgnoreSecretKey      bool
}

func Deploy(projectDir, imagesRepo, release, namespace, tag string, tagStrategy tag_strategy.TagStrategy, werfConfig *config.WerfConfig, opts DeployOptions) error {
	images := GetImagesInfoGetters(werfConfig.Images, imagesRepo, tag, false)

	m, err := GetSafeSecretManager(projectDir, opts.SecretValues, opts.IgnoreSecretKey)
	if err != nil {
		return err
	}

	serviceValues, err := GetServiceValues(werfConfig.Meta.Project, imagesRepo, namespace, tag, tagStrategy, images, ServiceValuesOptions{Env: opts.Env})
	if err != nil {
		return fmt.Errorf("error creating service values: %s", err)
	}

	serviceValuesRaw, _ := yaml.Marshal(serviceValues)
	logboek.LogInfoF("Using service values:\n%s", serviceValuesRaw)
	logboek.LogOptionalLn()

	werfChart, err := PrepareWerfChart(GetTmpWerfChartPath(werfConfig.Meta.Project), werfConfig.Meta.Project, projectDir, opts.Env, m, opts.SecretValues, serviceValues)
	if err != nil {
		return err
	}
	defer ReleaseTmpWerfChart(werfChart.ChartDir)

	werfChart.MergeExtraAnnotations(opts.UserExtraAnnotations)
	werfChart.MergeExtraLabels(opts.UserExtraLabels)
	werfChart.LogExtraAnnotations()
	werfChart.LogExtraLabels()

	logboek.LogOptionalLn()
	if err := helm.WithExtra(werfChart.ExtraAnnotations, werfChart.ExtraLabels, func() error {
		return werfChart.Deploy(release, namespace, helm.ChartOptions{
			Timeout: opts.Timeout,
			ChartValuesOptions: helm.ChartValuesOptions{
				Set:       opts.Set,
				SetString: opts.SetString,
				Values:    opts.Values,
			},
		})
	}); err != nil {
		replaceOld := fmt.Sprintf("%s/", werfChart.Name)
		replaceNew := fmt.Sprintf("%s/", ".helm")
		errMsg := strings.Replace(err.Error(), replaceOld, replaceNew, -1)
		return errors.New(errMsg)
	}

	return nil
}
