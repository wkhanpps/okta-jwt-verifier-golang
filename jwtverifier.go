/*******************************************************************************
 * Copyright 2018 Okta, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 ******************************************************************************/

package jwtverifier

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/okta/okta-jwt-verifier-golang/adaptors"
	"github.com/okta/okta-jwt-verifier-golang/adaptors/lestrratGoJwx"
	"github.com/okta/okta-jwt-verifier-golang/discovery"
	"github.com/okta/okta-jwt-verifier-golang/discovery/oidc"
	"github.com/okta/okta-jwt-verifier-golang/errors"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type JwtVerifier struct {
	Issuer string

	Leeway int64

	ClaimsToValidate map[string]string

	Discovery discovery.Discovery

	Adaptor adaptors.Adaptor
}

type Jwt struct {
	Claims map[string]interface{}
}

func (j *JwtVerifier) New() *JwtVerifier {
	// Default to OIDC discovery if none is defined
	if j.Discovery == nil {
		disc := oidc.Oidc{}
		j.Discovery = disc.New()
	}

	// Default to LestrratGoJwx Adaptor if none is defined
	if j.Adaptor == nil {
		adaptor := lestrratGoJwx.LestrratGoJwx{}
		j.Adaptor = adaptor.New()
	}

	// Default to PT2M Leeway
	j.Leeway = 120

	return j
}

func (j *JwtVerifier) VerifyAccessToken(jwt string) (*Jwt, error) {
	validJwt, err := j.isValidJwt(jwt)
	if validJwt == false {
		return nil, fmt.Errorf("token is not valid: %s", err)
	}

	resp, err := j.decodeJwt(jwt)
	if err != nil {
		return nil, err
	}

	token := resp.(map[string]interface{})

	myJwt := Jwt{
		Claims: token,
	}

	err = j.validateIss(token["iss"])
	if err != nil {
		return &myJwt, fmt.Errorf("the `Issuer` was not able to be validated. %s", err)
	}

	err = j.validateAudience(token["aud"])
	if err != nil {
		return &myJwt, fmt.Errorf("the `Audience` was not able to be validated. %s", err)
	}

	err = j.validateClientId(token["cid"])
	if err != nil {
		return &myJwt, fmt.Errorf("the `Client Id` was not able to be validated. %s", err)
	}

	err = j.validateExp(token["exp"])
	if err != nil {
		return &myJwt, fmt.Errorf("the `Expiration` was not able to be validated. %s", err)
	}

	err = j.validateExp(token["iat"])
	if err != nil {
		return &myJwt, fmt.Errorf("the `Issued At` was not able to be validated. %s", err)
	}

	return &myJwt, nil
}

func (j *JwtVerifier) decodeJwt(jwt string) (interface{}, error) {
	metaData, err := j.getMetaData()
	if err != nil {
		return nil, err
	}

	resp, err := j.Adaptor.Decode(jwt, metaData["jwks_uri"].(string))

	if err != nil {
		return nil, fmt.Errorf("could not decode token: %s", err)
	}

	return resp, nil
}

func (j *JwtVerifier) VerifyIdToken(jwt string) (*Jwt, error) {
	validJwt, err := j.isValidJwt(jwt)
	if validJwt == false {
		return nil, err
	}

	resp, err := j.decodeJwt(jwt)
	if err != nil {
		return nil, err
	}

	token := resp.(map[string]interface{})

	myJwt := Jwt{
		Claims: token,
	}

	err = j.validateIss(token["iss"])
	if err != nil {
		return &myJwt, fmt.Errorf("the `Issuer` was not able to be validated. %s", err)
	}

	err = j.validateAudience(token["aud"])
	if err != nil {
		return &myJwt, fmt.Errorf("the `Audience` was not able to be validated. %s", err)
	}

	err = j.validateExp(token["exp"])
	if err != nil {
		return &myJwt, fmt.Errorf("the `Expiration` was not able to be validated. %s", err)
	}

	err = j.validateExp(token["iat"])
	if err != nil {
		return &myJwt, fmt.Errorf("the `Issued At` was not able to be validated. %s", err)
	}

	err = j.validateNonce(token["nonce"])
	if err != nil {
		return &myJwt, fmt.Errorf("the `Nonce` was not able to be validated. %s", err)
	}

	return &myJwt, nil
}

func (j *JwtVerifier) GetDiscovery() discovery.Discovery {
	return j.Discovery
}

func (j *JwtVerifier) GetAdaptor() adaptors.Adaptor {
	return j.Adaptor
}

func (j *JwtVerifier) validateNonce(nonce interface{}) error {
	if j.ClaimsToValidate["nonce"] == "" {
		return nil
	}
	if nonce != j.ClaimsToValidate["nonce"] {
		return fmt.Errorf("nonce: %s does not match %s", nonce, j.ClaimsToValidate["nonce"])
	}
	return nil
}

func (j *JwtVerifier) validateAudience(audience interface{}) error {
	if j.ClaimsToValidate["aud"] == "" {
		return nil
	}
	if audience != j.ClaimsToValidate["aud"] {
		return fmt.Errorf("aud: %s does not match %s", audience, j.ClaimsToValidate["aud"])
	}
	return nil
}

func (j *JwtVerifier) validateClientId(clientId interface{}) error {
	if j.ClaimsToValidate["cid"] == "" {
		return nil
	}

	if clientId != j.ClaimsToValidate["cid"] {
		return fmt.Errorf("clientId: %s does not match %s", clientId, j.ClaimsToValidate["cid"])
	}
	return nil
}

func (j *JwtVerifier) validateExp(exp interface{}) error {
	if float64(time.Now().Unix() - j.Leeway) > exp.(float64) {
		return fmt.Errorf("the token is expired")
	}
	return nil
}

func (j *JwtVerifier) validateIat(iat interface{}) error {
	if float64(time.Now().Unix() + j.Leeway) < iat.(float64) {
		return fmt.Errorf("the token was issued in the future")
	}
	return nil
}

func (j *JwtVerifier) validateIss(issuer interface{}) error {
	if j.ClaimsToValidate["iss"] == "" {
		return nil
	}

	if issuer != j.ClaimsToValidate["iss"] {
		return fmt.Errorf("iss: %s does not match %s", issuer, j.ClaimsToValidate["iss"])
	}
	return nil
}

func (j *JwtVerifier) getMetaData() (map[string]interface{}, error) {
	metaDataUrl := j.Issuer + j.Discovery.GetWellKnownUrl()

	resp, err := http.Get(metaDataUrl)

	if err != nil {
		log.Fatal(err)
		return nil, fmt.Errorf("request for metadata was not successful: %s", err)
	}

	defer resp.Body.Close()

	md := make(map[string]interface{})
	json.NewDecoder(resp.Body).Decode(&md)

	return md, nil
}

func (j *JwtVerifier) isValidJwt(jwt string) (bool, error) {
	if jwt == "" {
		return false, errors.JwtEmptyStringError()
	}

	// Verify that the JWT contains at least one period ('.') character.
	var jwtRegex = regexp.MustCompile(`[a-zA-Z0-9-_]+\.[a-zA-Z0-9-_]+\.?([a-zA-Z0-9-_]+)[/a-zA-Z0-9-_]+?$`).MatchString
	if !jwtRegex(jwt) {
		return false, nil
	}

	parts := strings.Split(jwt, ".")
	header := parts[0]
	header = padHeader(header)
	headerDecoded, err := base64.StdEncoding.DecodeString(header)

	if err != nil {
		return false, nil
	}

	var jsonObject map[string]interface{}
	isHeaderJson := json.Unmarshal([]byte(headerDecoded), &jsonObject) == nil
	if isHeaderJson == false {
		return false, nil
	}

	if len(jsonObject) != 2 {
		return false, nil
	}

	_, algExists := jsonObject["alg"]
	_, kidExists := jsonObject["kid"]

	if algExists == false || kidExists == false {
		return false, nil
	}

	if jsonObject["alg"] != "RS256" {
		return false, nil
	}

	return true, nil
}
func padHeader(header string) string {
	if i := len(header) % 4; i != 0 {
		header += strings.Repeat("=", 4-i)
	}
	return header
}
