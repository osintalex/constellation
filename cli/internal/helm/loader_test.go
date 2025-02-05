/*
Copyright (c) Edgeless Systems GmbH

SPDX-License-Identifier: AGPL-3.0-only
*/

package helm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"testing"

	"github.com/edgelesssys/constellation/v2/internal/attestation/measurements"
	"github.com/edgelesssys/constellation/v2/internal/cloud/cloudprovider"
	"github.com/edgelesssys/constellation/v2/internal/config"
	"github.com/edgelesssys/constellation/v2/internal/deploy/helm"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/engine"
)

// TestLoad checks if the serialized format that Load returns correctly preserves the dependencies of the loaded chart.
func TestLoad(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	chartLoader := ChartLoader{}
	config := &config.Config{Provider: config.ProviderConfig{GCP: &config.GCPConfig{}}}
	release, err := chartLoader.Load(config, true, []byte("secret"), []byte("salt"))
	require.NoError(err)

	var helmReleases helm.Releases
	err = json.Unmarshal(release, &helmReleases)
	require.NoError(err)
	reader := bytes.NewReader(helmReleases.ConstellationServices.Chart)
	chart, err := loader.LoadArchive(reader)
	require.NoError(err)
	assert.NotNil(chart.Dependencies())
}

// TestConstellationServices checks if the rendered constellation-services chart produces the expected yaml files.
func TestConstellationServices(t *testing.T) {
	testCases := map[string]struct {
		config             *config.Config
		enforceIDKeyDigest bool
		valuesModifier     func(map[string]any) error
		ccmImage           string
		cnmImage           string
	}{
		"GCP": {
			config: &config.Config{Provider: config.ProviderConfig{GCP: &config.GCPConfig{
				DeployCSIDriver: func() *bool { b := true; return &b }(),
			}}},
			enforceIDKeyDigest: false,
			valuesModifier:     prepareGCPValues,
			ccmImage:           "ccmImageForGCP",
		},
		"Azure": {
			config: &config.Config{Provider: config.ProviderConfig{Azure: &config.AzureConfig{
				DeployCSIDriver:    func() *bool { b := true; return &b }(),
				EnforceIDKeyDigest: func() *bool { b := true; return &b }(),
			}}},
			enforceIDKeyDigest: true,
			valuesModifier:     prepareAzureValues,
			ccmImage:           "ccmImageForAzure",
			cnmImage:           "cnmImageForAzure",
		},
		"QEMU": {
			config:             &config.Config{Provider: config.ProviderConfig{QEMU: &config.QEMUConfig{}}},
			enforceIDKeyDigest: false,
			valuesModifier:     prepareQEMUValues,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			assert := assert.New(t)
			require := require.New(t)

			chartLoader := ChartLoader{
				joinServiceImage:         "joinServiceImage",
				kmsImage:                 "kmsImage",
				ccmImage:                 tc.ccmImage,
				cnmImage:                 tc.cnmImage,
				autoscalerImage:          "autoscalerImage",
				verificationServiceImage: "verificationImage",
			}
			chart, values, err := chartLoader.loadConstellationServicesHelper(tc.config, []byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), []byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))
			require.NoError(err)

			options := chartutil.ReleaseOptions{
				Name:      "testRelease",
				Namespace: "testNamespace",
				Revision:  1,
				IsInstall: true,
				IsUpgrade: false,
			}
			caps := &chartutil.Capabilities{}

			err = tc.valuesModifier(values)
			require.NoError(err)

			// This step is needed to enabled/disable subcharts according to their tags/conditions.
			err = chartutil.ProcessDependencies(chart, values)
			require.NoError(err)

			valuesToRender, err := chartutil.ToRenderValues(chart, values, options, caps)
			require.NoError(err)

			result, err := engine.Render(chart, valuesToRender)
			require.NoError(err)
			for k, v := range result {
				currentFile := path.Join("testdata", tc.config.GetProvider().String(), k)
				content, err := os.ReadFile(currentFile)

				// If a file does not exist, we expect the render for that path to be empty.
				if errors.Is(err, fs.ErrNotExist) {
					assert.YAMLEq("", v, fmt.Sprintf("current file: %s", currentFile))
					continue
				}
				assert.NoError(err)
				assert.YAMLEq(string(content), v, fmt.Sprintf("current file: %s", currentFile))
			}
		})
	}
}

// TestOperators checks if the rendered constellation-services chart produces the expected yaml files.
func TestOperators(t *testing.T) {
	testCases := map[string]struct {
		csp cloudprovider.Provider
	}{
		"GCP": {
			csp: cloudprovider.GCP,
		},
		"Azure": {
			csp: cloudprovider.Azure,
		},
		"QEMU": {
			csp: cloudprovider.QEMU,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			assert := assert.New(t)
			require := require.New(t)

			chartLoader := ChartLoader{joinServiceImage: "joinServiceImage", kmsImage: "kmsImage", ccmImage: "ccmImage", cnmImage: "cnmImage", autoscalerImage: "autoscalerImage"}
			chart, vals, err := chartLoader.loadOperatorsHelper(tc.csp)
			require.NoError(err)

			options := chartutil.ReleaseOptions{
				Name:      "testRelease",
				Namespace: "testNamespace",
				Revision:  1,
				IsInstall: true,
				IsUpgrade: false,
			}
			caps := &chartutil.Capabilities{}

			conOpVals, ok := vals["constellation-operator"].(map[string]any)
			require.True(ok)
			conOpVals["constellationUID"] = "42424242424242"

			// This step is needed to enabled/disable subcharts according to their tags/conditions.
			err = chartutil.ProcessDependencies(chart, vals)
			require.NoError(err)

			valuesToRender, err := chartutil.ToRenderValues(chart, vals, options, caps)
			require.NoError(err)

			result, err := engine.Render(chart, valuesToRender)
			require.NoError(err)
			for k, v := range result {
				currentFile := path.Join("testdata", tc.csp.String(), k)
				content, err := os.ReadFile(currentFile)

				// If a file does not exist, we expect the render for that path to be empty.
				if errors.Is(err, fs.ErrNotExist) {
					assert.YAMLEq("", v, fmt.Sprintf("current file: %s", currentFile))
					continue
				}
				assert.NoError(err)
				assert.YAMLEq(string(content), v, fmt.Sprintf("current file: %s", currentFile))
			}
		})
	}
}

func prepareGCPValues(values map[string]any) error {
	joinVals, ok := values["join-service"].(map[string]any)
	if !ok {
		return errors.New("missing 'join-service' key")
	}

	m := measurements.M{
		1: measurements.WithAllBytes(0xAA, false),
	}
	mJSON, err := json.Marshal(m)
	if err != nil {
		return err
	}
	joinVals["measurements"] = string(mJSON)
	joinVals["measurementSalt"] = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

	ccmVals, ok := values["ccm"].(map[string]any)
	if !ok {
		return errors.New("missing 'ccm' key")
	}
	ccmVals["GCP"].(map[string]any)["subnetworkPodCIDR"] = "192.0.2.0/24"
	ccmVals["GCP"].(map[string]any)["projectID"] = "42424242424242"
	ccmVals["GCP"].(map[string]any)["uid"] = "242424242424"
	ccmVals["GCP"].(map[string]any)["secretData"] = "baaaaaad"

	testTag := "v0.0.0"
	pullPolicy := "IfNotPresent"
	csiVals, ok := values["gcp-compute-persistent-disk-csi-driver"].(map[string]any)
	if !ok {
		return errors.New("missing 'gcp-compute-persistent-disk-csi-driver' key")
	}
	csiVals["image"] = map[string]any{
		"csiProvisioner": map[string]any{
			"repo":       "csi-provisioner",
			"tag":        testTag,
			"pullPolicy": pullPolicy,
		},
		"csiAttacher": map[string]any{
			"repo":       "csi-attacher",
			"tag":        testTag,
			"pullPolicy": pullPolicy,
		},
		"csiResizer": map[string]any{
			"repo":       "csi-resizer",
			"tag":        testTag,
			"pullPolicy": pullPolicy,
		},
		"csiSnapshotter": map[string]any{
			"repo":       "csi-snapshotter",
			"tag":        testTag,
			"pullPolicy": pullPolicy,
		},
		"csiNodeRegistrar": map[string]any{
			"repo":       "csi-registrar",
			"tag":        testTag,
			"pullPolicy": pullPolicy,
		},
		"gcepdDriver": map[string]any{
			"repo":       "csi-driver",
			"tag":        testTag,
			"pullPolicy": pullPolicy,
		},
	}

	verificationVals, ok := values["verification-service"].(map[string]any)
	if !ok {
		return errors.New("missing 'verification-service' key")
	}
	verificationVals["loadBalancerIP"] = "127.0.0.1"

	konnectivityVals, ok := values["konnectivity"].(map[string]any)
	if !ok {
		return errors.New("missing 'konnectivity' key")
	}
	konnectivityVals["loadBalancerIP"] = "127.0.0.1"

	return nil
}

func prepareAzureValues(values map[string]any) error {
	joinVals, ok := values["join-service"].(map[string]any)
	if !ok {
		return errors.New("missing 'join-service' key")
	}
	joinVals["idkeydigest"] = "baaaaaadbaaaaaadbaaaaaadbaaaaaadbaaaaaadbaaaaaadbaaaaaadbaaaaaad"
	m := measurements.M{1: measurements.WithAllBytes(0xAA, false)}
	mJSON, err := json.Marshal(m)
	if err != nil {
		return err
	}
	joinVals["measurements"] = string(mJSON)
	joinVals["measurementSalt"] = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

	ccmVals, ok := values["ccm"].(map[string]any)
	if !ok {
		return errors.New("missing 'ccm' key")
	}
	ccmVals["Azure"].(map[string]any)["subnetworkPodCIDR"] = "192.0.2.0/24"
	ccmVals["Azure"].(map[string]any)["azureConfig"] = "baaaaaad"

	autoscalerVals, ok := values["autoscaler"].(map[string]any)
	if !ok {
		return errors.New("missing 'autoscaler' key")
	}
	autoscalerVals["Azure"] = map[string]any{
		"clientID":       "AppClientID",
		"clientSecret":   "ClientSecretValue",
		"resourceGroup":  "resourceGroup",
		"subscriptionID": "subscriptionID",
		"tenantID":       "TenantID",
	}

	verificationVals, ok := values["verification-service"].(map[string]any)
	if !ok {
		return errors.New("missing 'verification-service' key")
	}
	verificationVals["loadBalancerIP"] = "127.0.0.1"

	konnectivityVals, ok := values["konnectivity"].(map[string]any)
	if !ok {
		return errors.New("missing 'konnectivity' key")
	}
	konnectivityVals["loadBalancerIP"] = "127.0.0.1"

	return nil
}

func prepareQEMUValues(values map[string]any) error {
	joinVals, ok := values["join-service"].(map[string]any)
	if !ok {
		return errors.New("missing 'join-service' key")
	}
	m := measurements.M{1: measurements.WithAllBytes(0xAA, false)}
	mJSON, err := json.Marshal(m)
	if err != nil {
		return err
	}
	joinVals["measurements"] = string(mJSON)
	joinVals["measurementSalt"] = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

	verificationVals, ok := values["verification-service"].(map[string]any)
	if !ok {
		return errors.New("missing 'verification-service' key")
	}
	verificationVals["loadBalancerIP"] = "127.0.0.1"

	konnectivityVals, ok := values["konnectivity"].(map[string]any)
	if !ok {
		return errors.New("missing 'konnectivity' key")
	}
	konnectivityVals["loadBalancerIP"] = "127.0.0.1"

	return nil
}
