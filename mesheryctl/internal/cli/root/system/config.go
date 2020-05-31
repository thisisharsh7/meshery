// Copyright 2020 The Meshery Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package system

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"

	"github.com/layer5io/meshery/mesheryctl/pkg/utils"
	log "github.com/sirupsen/logrus"

	"github.com/spf13/cobra"
)

const URL1 = "http://localhost:9081/api/k8sconfig/contexts"
const URL2 = "http://localhost:9081/api/k8sconfig"

const tokenName = "token"
const providerName = "meshery-provider"
const paramName = "k8sfile"
const contextName = "contextName"

var tokenPath string

func UploadFileWithParams(uri string, params map[string]string, paramName, path string) (*http.Request, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	fileContents, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, err
	}
	fi, err := file.Stat()
	if err != nil {
		return nil, err
	}
	file.Close()

	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile(paramName, fi.Name())
	if err != nil {
		return nil, err
	}
	part.Write(fileContents)

	for key, val := range params {
		_ = writer.WriteField(key, val)
	}
	err = writer.Close()
	if err != nil {
		return nil, err
	}

	request, err := http.NewRequest("POST", uri, body)
	if err != nil {
		return nil, err
	}
	request.Header.Add("Content-Type", writer.FormDataContentType())
	return request, nil
}

func getContexts(configFile, tokenPath string) ([]string, error) {
	client := &http.Client{}

	req, err := UploadFileWithParams(URL1, nil, paramName, configFile)
	if err != nil {
		return nil, err
	}

	err = utils.AddAuthDetails(req, tokenPath)
	if err != nil {
		return nil, err
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	var val []map[string]string
	json.Unmarshal(body, &val)

	var contexts []string
	for _, item := range val {
		contexts = append(contexts, item["contextName"])
	}
	return contexts, nil
}

// TODO: show used the available context and let user choose which context to set
func SetDefaultContext(configFile, cname, tokenPath string) error {
	client := &http.Client{}
	extraParams1 := map[string]string{
		"contextName": cname,
	}
	req, err := UploadFileWithParams(URL2, extraParams1, paramName, configFile)
	if err != nil {
		return err
	}
	err = utils.AddAuthDetails(req, tokenPath)
	if err != nil {
		return err
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}
	// TODO: Pretty print the output
	fmt.Printf("%v\n", string(body))
	return nil
}

// resetCmd represents the config command
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configure Meshery",
	Long:  `Configure the Kubernetes cluster used by Meshery.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {

		if tokenPath == "" {
			log.Fatal("Token path invalid")
		}

		switch args[0] {
		case "minikube":
			generateCFG := exec.Command("sh", "-c", utils.ScriptMinikube())
			generateCFG.Stdout = os.Stdout
			generateCFG.Stderr = os.Stderr

			if err := generateCFG.Run(); err != nil {
				log.Fatal("Error generating config:", err)
				return
			}
		case "gke":
			saName := "sa-meshery-" + utils.StringWithCharset(8)
			generateCFG := exec.Command("sh", "-c", utils.ScriptGKE(saName, "default"))
			generateCFG.Stdout = os.Stdout
			generateCFG.Stderr = os.Stderr

			if err := generateCFG.Run(); err != nil {
				log.Fatal("Error generating config:", err)
				return
			}
		default:
			log.Fatal("The argument has to be one of GKE | Minikube")
		}
		configPath := "/tmp/meshery/kubeconfig.yaml"

		contexts, err := getContexts(configPath, tokenPath)
		if err != nil || len(contexts) < 1 {
			log.Fatalf("Error getting contexts : %s", err.Error())
		}

		choosenCtx := contexts[0]
		if len(contexts) > 1 {
			fmt.Println("List of available contexts : ")
			for i, ctx := range contexts {
				fmt.Printf("(%d) %s \n", i+1, ctx)
			}
			var choice int
			fmt.Print("Enter choice: ")
			_, err = fmt.Scanf("%d", &choice)
			choosenCtx = contexts[choice-1]
		}

		log.Debugf("Choosen context : %s", choosenCtx)
		err = SetDefaultContext(configPath, choosenCtx, tokenPath)
		if err != nil {
			log.Fatalf("Error setting context : %s", err.Error())
		}
	},
}

func init() {
	configCmd.Flags().StringVar(&tokenPath, "token", utils.AuthConfigFile, "(optional) Path to meshery auth config")
}
