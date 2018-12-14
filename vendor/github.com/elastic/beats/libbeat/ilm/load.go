package ilm

import (
	"encoding/json"
	"fmt"

	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/libbeat/template"
)

type ESClient interface {
	LoadJSON(path string, json map[string]interface{}) ([]byte, error)
	Request(method, path string, pipeline string, params map[string]string, body interface{}) (int, []byte, error)
	GetVersion() common.Version
}

type Loader struct {
	esClient         ESClient
	beatInfo         beat.Info
	esVersion        common.Version
	ilmPolicyConfigs []ilmPolicyCfg
}

type ilmPolicyCfg struct {
	idxName    string
	policyName string
}

const pattern = "000001"

// NewESLoader creates a new Elasticsearch ilm policy loader
func NewESLoader(outCfg *common.Config, client ESClient, beatInfo beat.Info) (*Loader, error) {
	// Get ES Index name for comparison
	esCfg := struct {
		Index          string                   `config:"index"`
		RolloverPolicy string                   `config:"rollover_policy"`
		Indices        []map[string]interface{} `config:"indices"`
	}{}
	err := outCfg.Unpack(&esCfg)
	if err != nil {
		return nil, err
	}
	var ilmPolicyConfigs []ilmPolicyCfg
	if esCfg.Index != "" && esCfg.RolloverPolicy != "" {
		ilmPolicyConfigs = append(ilmPolicyConfigs, ilmPolicyCfg{idxName: esCfg.Index, policyName: esCfg.RolloverPolicy})
	}
	if esCfg.Indices != nil {
		for _, idxCfg := range esCfg.Indices {
			if policy, ok := idxCfg["rollover_policy"]; ok {
				//TODO: check if error checks necessary
				ilmPolicyConfigs = append(ilmPolicyConfigs, ilmPolicyCfg{idxName: idxCfg["index"].(string), policyName: policy.(string)})
			}
		}
	}

	return &Loader{
		esClient:         client,
		ilmPolicyConfigs: ilmPolicyConfigs,
		beatInfo:         beatInfo,
		esVersion:        client.GetVersion(),
	}, nil
}

func (l *Loader) LoadPolicies() error {
	for _, policyCfg := range l.ilmPolicyConfigs {
		if policy, ok := policies[policyCfg.policyName]; ok {
			_, _, err := l.esClient.Request("PUT", "/_ilm/policy/"+policyCfg.policyName, "", nil, policy)
			return err
		} else {
			logp.Warn("Can not find policy %s", policyCfg.policyName)
		}
	}
	return nil
}

func (l *Loader) LoadWriteAlias() error {
	// Check that ILM is enabled and the right elasticsearch version exists
	if err := ilmEnabled(l.esClient); err != nil {
		return err
	}

	if len(l.ilmPolicyConfigs) > 0 {
		logp.Warn("index Life Cycle Management is in beta!")
	}

	for _, policyCfg := range l.ilmPolicyConfigs {
		//TODO: check index name
		rolloverAlias := policyCfg.idxName

		// TODO: either remove or let pattern be configurable. This always assume it's a date pattern by sourrounding it by <...>
		firstIndex := fmt.Sprintf("%%3C%s-%s%%3E", rolloverAlias, pattern)

		// Check if alias already exists
		status, b, err := l.esClient.Request("HEAD", "/_alias/"+rolloverAlias, "", nil, nil)
		if err != nil && status != 404 {
			logp.Err("Failed to check for alias: %s: %+v", err, string(b))
			continue
			//return errw.Wrap(err, "failed to check for alias")
		}
		if status == 200 {
			logp.Info("Write alias already exists")
			continue
		}

		body := common.MapStr{
			"aliases": common.MapStr{
				rolloverAlias: common.MapStr{
					"is_write_index": true,
				},
			},
		}

		// Create alias with write index
		code, res, err := l.esClient.Request("PUT", "/"+firstIndex, "", nil, body)
		if code == 400 {
			logp.Err("Error creating alias with write index. As return code is 400, assuming already exists: %s, %s", err, string(res))
			continue

		} else if err != nil {
			logp.Err("Error creating alias with write index: %s, %s", err, string(res))
			continue
			//return errw.Wrap(err, "failed to create write alias: "+string(res))
		}

		logp.Info("Alias with write index created: %s", firstIndex)

		//register additional template
		ilmTemplate := createILMTemplate(policyCfg)
		templateLoader, err := template.NewESLoader(&common.Config{}, l.esClient, l.beatInfo)
		if err != nil {
			logp.Err("Error loading template for alias: %s, %s", policyCfg.idxName, err)
			continue
		}
		templateLoader.LoadTemplate(policyCfg.idxName, ilmTemplate)
	}

	return nil
}

func createILMTemplate(policyCfg ilmPolicyCfg) common.MapStr {
	return common.MapStr{
		"order":          2,
		"index_patterns": fmt.Sprintf("%s*", policyCfg.idxName),
		"settings": common.MapStr{
			"index": common.MapStr{
				"lifecycle": common.MapStr{
					"name":           policyCfg.policyName,
					"rollover_alias": policyCfg.idxName,
				},
			},
		},
	}
}

func ilmEnabled(client ESClient) error {
	if err := checkElasticsearchVersionIlm(client); err != nil {
		return err
	}
	return checkILMFeatureEnabled(client)
}

func checkElasticsearchVersionIlm(client ESClient) error {
	esV := client.GetVersion()
	requiredVersion, err := common.NewVersion("6.6.0")
	if err != nil {
		return err
	}

	if esV.LessThan(requiredVersion) {
		return fmt.Errorf("ILM requires at least Elasticsearch 6.6.0. Used version: %s", esV.String())
	}

	return nil
}

func checkILMFeatureEnabled(client ESClient) error {
	code, body, err := client.Request("GET", "/_xpack", "", nil, nil)

	// If we get a 400, it's assumed to be the OSS version of Elasticsearch
	if code == 400 {
		return fmt.Errorf("ILM feature is not available in this Elasticsearch version")
	}
	if err != nil {
		return err
	}

	var response struct {
		Features struct {
			ILM struct {
				Available bool `json:"available"`
				Enabled   bool `json:"enabled"`
			} `json:"ilm"`
		} `json:"features"`
	}

	err = json.Unmarshal(body, &response)
	if err != nil {
		return fmt.Errorf("failed to parse JSON response: %v", err)
	}

	if !response.Features.ILM.Available {
		return fmt.Errorf("ILM feature is not available in Elasticsearch")
	}

	if !response.Features.ILM.Enabled {
		return fmt.Errorf("ILM feature is not enabled in Elasticsearch")
	}

	return nil
}
