// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//     http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package command

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	generate "github.com/aws-controllers-k8s/code-generator/pkg/generate"
	ackgenerate "github.com/aws-controllers-k8s/code-generator/pkg/generate/ack"
	"github.com/aws-controllers-k8s/code-generator/pkg/model"
	"github.com/aws-controllers-k8s/code-generator/pkg/util"
)

type contentType int

const (
	ctUnknown contentType = iota
	ctJSON
	ctYAML
)

var (
	optGenVersion    string
	optAPIsInputPath string
	apisVersionPath  string
)

// apiCmd is the command that generates service API types
var apisCmd = &cobra.Command{
	Use:      "apis <service>",
	Short:    "Generate Kubernetes API type definitions for an AWS service API",
	RunE:     generateAPIs,
	PostRunE: saveGeneratedMetadata,
}

func init() {
	apisCmd.PersistentFlags().StringVar(
		&optGenVersion, "version", "v1alpha1", "the resource API Version to use when generating API infrastructure and type definitions",
	)
	rootCmd.AddCommand(apisCmd)
}

// saveGeneratedMetadata saves the parameters used to generate APIs and checksum
// of the generated code.
func saveGeneratedMetadata(cmd *cobra.Command, args []string) error {
	err := ackgenerate.CreateGenerationMetadata(
		optGenVersion,
		filepath.Join(optOutputPath, "apis"),
		ackgenerate.UpdateReasonAPIGeneration,
		optAWSSDKGoVersion,
		optGeneratorConfigPath,
	)
	if err != nil {
		return fmt.Errorf("cannot create generation metadata file: %v", err)
	}

	copyDest := filepath.Join(
		optOutputPath, "apis", optGenVersion, "generator.yaml",
	)
	err = util.CopyFile(optGeneratorConfigPath, copyDest)
	if err != nil {
		return fmt.Errorf("cannot copy generator configuration file: %v", err)
	}

	return nil
}

// generateAPIs generates the Go files for each resource in the AWS service
// API.
func generateAPIs(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("please specify the service alias for the AWS service API to generate")
	}
	svcAlias := strings.ToLower(args[0])
	if optOutputPath == "" {
		optOutputPath = filepath.Join(optServicesDir, svcAlias)
	}
	if err := ensureSDKRepo(optCacheDir); err != nil {
		return err
	}
	sdkHelper := model.NewSDKHelper(sdkDir)
	sdkAPI, err := sdkHelper.API(svcAlias)
	if err != nil {
		newSvcAlias, err := FallBackFindServiceID(sdkDir, svcAlias)
		if err != nil {
			return err
		}
		sdkAPI, err = sdkHelper.API(newSvcAlias) // retry with serviceID
		if err != nil {
			return fmt.Errorf("service %s not found", svcAlias)
		}
	}
	g, err := generate.New(
		sdkAPI, optGenVersion, optGeneratorConfigPath, ackgenerate.DefaultConfig,
	)
	if err != nil {
		return err
	}
	ts, err := ackgenerate.APIs(g, optTemplateDirs)
	if err != nil {
		return err
	}

	if err = ts.Execute(); err != nil {
		return err
	}

	apisVersionPath = filepath.Join(optOutputPath, "apis", optGenVersion)
	for path, contents := range ts.Executed() {
		if optDryRun {
			fmt.Printf("============================= %s ======================================\n", path)
			fmt.Println(strings.TrimSpace(contents.String()))
			continue
		}
		outPath := filepath.Join(apisVersionPath, path)
		outDir := filepath.Dir(outPath)
		if _, err := ensureDir(outDir); err != nil {
			return err
		}
		if err = ioutil.WriteFile(outPath, contents.Bytes(), 0666); err != nil {
			return err
		}
	}
	return nil
}
