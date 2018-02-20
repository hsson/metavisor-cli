package wrap

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"

	"github.com/brkt/metavisor-cli/pkg/logging"
	"github.com/brkt/metavisor-cli/pkg/userdata"
)

const (
	compressUserdata  = true
	servicePort       = 443
	modeMetavisor     = "metavisor"
	configContentType = "text/brkt-config"
)

type instanceConfig struct {
	AllowUnencrypyed bool   `json:"allow_unencrypted_guest"`
	APIHost          string `json:"api_host,omitempty"`
	HsmproxyHost     string `json:"hsmproxy_host,omitempty"`
	IdentityToken    string `json:"identity_token,omitempty"`
	NetworkHost      string `json:"network_host,omitempty"`
	SoloMode         string `json:"solo_mode"`
}

func (i instanceConfig) ToJSON() string {
	ic := struct {
		Config instanceConfig `json:"brkt"`
	}{i}
	b, _ := json.Marshal(ic)
	return string(b)
}

func configFromDomain(domain string) instanceConfig {
	return instanceConfig{
		APIHost:      fmt.Sprintf("yetiapi.%s:%d", domain, servicePort),
		HsmproxyHost: fmt.Sprintf("hsmproxy.%s:%d", domain, servicePort),
		NetworkHost:  fmt.Sprintf("network.%s:%d", domain, servicePort),
	}
}

func generateUserdataString(launchToken, domain string, compress bool) (string, error) {
	conf := configFromDomain(domain)
	conf.AllowUnencrypyed = true
	conf.SoloMode = modeMetavisor
	// It's important to log before setting the token, as it's sensitive
	logging.Debugf("Parsed instance config: %s", conf.ToJSON())
	if launchToken != "" {
		// TODO: Validate that token is in fact a launch token, check token_type field
		conf.IdentityToken = launchToken
	}
	userDataContainer := userdata.New()
	userDataContainer.AddPart(configContentType, conf.ToJSON())
	userDataMIME := userDataContainer.ToMIMEText()
	if compress {
		compressed, err := compressString(userDataMIME)
		if err != nil {
			logging.Warning("Error while compressing userdata, using un-compressed instead")
		} else {
			userDataMIME = compressed
		}
	}
	return userDataMIME, nil
}

func compressString(input string) (string, error) {
	buffer := new(bytes.Buffer)
	writer := gzip.NewWriter(buffer)
	_, err := writer.Write([]byte(input))
	if err != nil {
		logging.Debugf("Got error when gzipping data: %s", err)
		return "", err
	}
	err = writer.Close()
	if err != nil {
		logging.Debugf("Got error when gzipping userdata: %s", err)
		return "", err
	}
	return buffer.String(), nil
}
