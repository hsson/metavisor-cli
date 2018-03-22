package wrap

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/immutable-systems/metavisor-cli/pkg/logging"
	"github.com/immutable-systems/metavisor-cli/pkg/userdata"
)

const (
	compressUserdata    = true
	servicePort         = 443
	modeMetavisor       = "metavisor"
	configContentType   = "text/brkt-config"
	tokenTypeKey        = "brkt.token_type"
	tokenTypeValidValue = "launch"
)

var (
	// ErrInvalidLaunchToken is returned if trying to use a token that's not valid
	ErrInvalidLaunchToken = errors.New("specified token is not a valid launch token")
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

// Manually decoding JWT to avoid additional dependencies
func isValidToken(token string) bool {
	tokenSlice := strings.Split(token, ".")
	if len(tokenSlice) != 3 {
		// JWT is three parts
		logging.Debug("Token does not seem to be a JWT")
		return false
	}
	// Payload is in middle part of JWT
	payload := tokenSlice[1]
	data, err := base64.RawStdEncoding.DecodeString(payload)
	if err != nil {
		logging.Debug("Token could not be raw base64 decoded")
		data, err = base64.StdEncoding.DecodeString(payload)
		if err != nil {
			// Unable to decode token
			logging.Debug("Token could not be base64 decoded")
			return false
		}
	}
	// Payload is map from string -> arbitrary value
	res := make(map[string]interface{})
	err = json.Unmarshal(data, &res)
	if err != nil {
		// Could not unmarshal payload
		logging.Debug("Token could not be unmarshaled to JSON")
		return false
	}
	tokenType, exist := res[tokenTypeKey]
	if !exist {
		// Payload has no token type
		logging.Debug("Token has no token type attribute")
		return false
	}
	tokenTypeString, ok := tokenType.(string)
	if !ok {
		// Token type is not a string
		logging.Debug("Token type is not of a valid type")
		return false
	}
	return strings.ToLower(tokenTypeString) == tokenTypeValidValue
}

func generateUserdataString(launchToken, domain string, compress bool) (string, error) {
	conf := configFromDomain(domain)
	conf.AllowUnencrypyed = true
	conf.SoloMode = modeMetavisor
	// It's important to log before setting the token, as it's sensitive
	logging.Debugf("Parsed instance config: %s", conf.ToJSON())
	if launchToken != "" {
		isValid := isValidToken(launchToken)
		if !isValid {
			// The specified token is not a valid launch token
			return "", ErrInvalidLaunchToken
		}
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
