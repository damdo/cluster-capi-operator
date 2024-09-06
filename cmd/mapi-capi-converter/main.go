// Copyright (202[1-9]|20[3-9][0-9]) Red Hat, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	configv1 "github.com/openshift/api/config/v1"
	mapiv1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cluster-capi-operator/pkg/capi2mapi"
	"github.com/openshift/cluster-capi-operator/pkg/mapi2capi"

	capav1 "sigs.k8s.io/cluster-api-provider-aws/v2/api/v1beta2"

	"github.com/go-test/deep"
)

func readCsvFile(filePath string) ([][]string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("Unable to read input file %q: %w", filePath, err)
	}
	defer f.Close()

	csvReader := csv.NewReader(f)
	records, err := csvReader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("Unable to parse file as CSV for %q: %w", filePath, err)
	}

	return records, nil
}

func main() {
	log.Println("loading test MachineSets from dataset...")
	records, err := readCsvFile("/Users/ddonati/Downloads/20240517_134742_02629_w5fy7.csv")
	if err != nil {
		log.Fatalf("unable to read csv file: %v", err)
	}

	infra := configv1.Infrastructure{
		Spec: configv1.InfrastructureSpec{},
		Status: configv1.InfrastructureStatus{
			InfrastructureName: "sample-cluster-name",
		},
	}

	capiAWSCluster := &capav1.AWSCluster{
		Spec: capav1.AWSClusterSpec{
			Region: "us-east-2",
		},
		Status: capav1.AWSClusterStatus{
			Ready: true,
		},
	}

	var machinesets []mapiv1.MachineSet
	for j, r := range records {
		for i, l := range r {
			// For now only use AWS MachineSets for the test.
			// TODO: remove this once other providers are supported.
			if l == "content" || !strings.Contains(l, "AWSMachineProviderConfig") {
				continue
			}

			var m mapiv1.MachineSet
			if err := json.Unmarshal([]byte(l), &m); err != nil {
				log.Printf("error unmarshalling dataset MachineSet %d %d", j, i)
				continue
			}
			machinesets = append(machinesets, m)
		}
	}

	var conversionSuccess int

	var conversionTotal int

	conversionFailures := []int{}

	for i, ms := range machinesets {
		// Process one test MachineSet at a time.

		// TODO: tmp: used this as a limiter.
		if i > 1000 {
			continue
		}

		fmt.Println("")
		log.Printf("converting test MachineSet %d", i)

		conversionTotal++

		// Extract providerSpec from the original MAPI MachineSet.
		mapiMsProviderSpec, err := mapi2capi.AWSProviderSpecFromRawExtension(ms.Spec.Template.Spec.ProviderSpec.Value)
		if err != nil {
			fmt.Printf("error converting to MAPI MachineSet Provider n. %d: %v\n", i, err)
			conversionFailures = append(conversionFailures, i)

			continue
		}

		// Convert original MAPI MachineSet to CAPI MachineSet and CAPI MachineTemplate.
		capiMachineSet, capiAWSTemplate, _, err := mapi2capi.FromAWSMachineSetAndInfra(&ms, &infra).ToMachineSetAndMachineTemplate()
		if err != nil {
			// spec := string(ms.Spec.Template.Spec.ProviderSpec.Value.Raw)
			fmt.Printf("error converting MAPI MachineSet to CAPI MachineSet and MachineTemplate  n. %d: %v\n", i, err)
			conversionFailures = append(conversionFailures, i)

			continue
		}

		// Convert CAPI MachineSet and CAPI MachineTemplate back to a "roundtripped" MAPI MachineSet.
		capiAWSCluster.Spec.Region = mapiMsProviderSpec.Placement.Region // Artificially set the Region from the initial MAPI ProviderSpec.

		newMAPIMs, _, err := capi2mapi.FromMachineSetAndAWSMachineTemplateAndAWSCluster(capiMachineSet, capiAWSTemplate, capiAWSCluster).ToMachineSet()
		if err != nil {
			fmt.Printf("error converting CAPI Machineset to MAPI MachineSet n. %d: %v\n", i, err)
			conversionFailures = append(conversionFailures, i)

			continue
		}

		// Extract providerSpec from the "roundtripped" MAPI MachineSet.
		newMAPIMsProviderSpec, err := mapi2capi.AWSProviderSpecFromRawExtension(newMAPIMs.Spec.Template.Spec.ProviderSpec.Value)
		if err != nil {
			fmt.Printf("error converting CAPI ProvideSpec to MAPI MachineSet Provider n. %d: %v\n", i, err)
			conversionFailures = append(conversionFailures, i)

			continue
		}

		// Diff the original MAPI MachineSets with the roundtripped MAPI MachineSets.
		msDiff := deep.Equal(&ms, newMAPIMs)
		if msDiff != nil {
			for _, d := range msDiff {
				if strings.HasPrefix(d, "Spec.Template.Spec.ProviderSpec.Value.Raw") /* ignore ProviderSpec diff, as that is handled later */ ||
					strings.HasPrefix(d, "ObjectMeta") /* ignore ObjectMeta for now */ {
					continue
				}
				fmt.Printf("-> %#v\n", d)
			}
		}

		// Diff the providerSpec of the original MAPI MachineSets with the providerSpec of the roundtripped MAPI MachineSets.
		providerSpecDiff := deep.Equal(mapiMsProviderSpec, newMAPIMsProviderSpec)
		if providerSpecDiff != nil {
			for _, d := range providerSpecDiff {
				if strings.HasPrefix(d, "Tags.slice") /* ignore Tags slice diff, as that is due to slice -> map -> slice conversion ordering issue */ ||
					strings.HasPrefix(d, "TypeMeta.APIVersion") {
					continue
				}
				fmt.Printf("-> %#v\n", d)
			}
		}

		// yamlCAPIMachineSet, err := yaml.Marshal(capiMachineSet)
		// if err != nil {
		// 	log.Println(err)
		// }

		// yamlCAPIAWSTemplate, err := yaml.Marshal(capiAWSTemplate)
		// if err != nil {
		// 	log.Println(err)
		// }

		// fmt.Println("###################")
		// fmt.Printf("%s\n", string(yamlCAPIMachineSet))
		// fmt.Println("----")
		// fmt.Printf("%s\n", string(yamlCAPIAWSTemplate))
		// fmt.Println("###################")

		// jsonCAPIMachineSet, err := json.MarshalIndent(capiMachineSet, " ", " ")
		// if err != nil {
		// 	log.Println(err)
		// }

		// jsonCAPIAWSTemplate, err := json.MarshalIndent(capiAWSTemplate, " ", " ")
		// if err != nil {
		// 	log.Println(err)
		// }

		// fmt.Println("###################")
		// fmt.Printf("%s\n", string(jsonCAPIMachineSet))
		// fmt.Println("----")
		// fmt.Printf("%s\n", string(jsonCAPIAWSTemplate))
		// fmt.Println("###################")

		conversionSuccess++
	}

	fmt.Println("")
	fmt.Println("Conversion Results")
	fmt.Printf("Successes: %d  (includes with diff)\n", conversionSuccess)
	fmt.Printf("Failures:  %d  (following machinesets failed: %v)\n", len(conversionFailures), conversionFailures)
	fmt.Printf("Total:     %d\n", conversionTotal)
}
