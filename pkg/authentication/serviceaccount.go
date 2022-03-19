package authentication

import (
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/request/bearertoken"
	unionauth "k8s.io/apiserver/pkg/authentication/request/union"
	"k8s.io/apiserver/pkg/authentication/request/websocket"
	tokenunion "k8s.io/apiserver/pkg/authentication/token/union"
	"k8s.io/client-go/util/keyutil"
	"k8s.io/kubernetes/pkg/serviceaccount"
)

// NewUnsafeNonLookupServiceAccountAuthenticator creates an service account authenticator that is not looking up
// service accounts and is hence unsafe for real authentication. But the returned authentication response
// can be used for non-critical purposes.
func NewUnsafeNonLookupServiceAccountAuthenticator(keyfiles []string, boundIssuers []string, apiAudiences authenticator.Audiences) (authenticator.Request, error) {
	legacy, err := newLegacyServiceAccountAuthenticator(keyfiles, false, apiAudiences, nil)
	if err != nil {
		return nil, err
	}
	bound, err := newBoundServiceAccountAuthenticator(boundIssuers, keyfiles, apiAudiences, nil)
	if err != nil {
		return nil, err
	}
	tokenAuth := tokenunion.New(legacy, bound)
	return unionauth.New(bearertoken.New(tokenAuth), websocket.NewProtocolAuthenticator(tokenAuth)), nil
}

// newLegacyServiceAccountAuthenticator returns an authenticator.Token or an error
func newLegacyServiceAccountAuthenticator(keyfiles []string, lookup bool, apiAudiences authenticator.Audiences, serviceAccountGetter serviceaccount.ServiceAccountTokenGetter) (authenticator.Token, error) {
	allPublicKeys := []interface{}{}
	for _, keyfile := range keyfiles {
		publicKeys, err := keyutil.PublicKeysFromFile(keyfile)
		if err != nil {
			return nil, err
		}
		allPublicKeys = append(allPublicKeys, publicKeys...)
	}

	tokenAuthenticator := serviceaccount.JWTTokenAuthenticator([]string{serviceaccount.LegacyIssuer}, allPublicKeys, apiAudiences, serviceaccount.NewLegacyValidator(lookup, serviceAccountGetter))
	return tokenAuthenticator, nil
}

// newBoundServiceAccountAuthenticator returns an authenticator.Token or an error
func newBoundServiceAccountAuthenticator(issuers []string, keyfiles []string, apiAudiences authenticator.Audiences, serviceAccountGetter serviceaccount.ServiceAccountTokenGetter) (authenticator.Token, error) {
	allPublicKeys := []interface{}{}
	for _, keyfile := range keyfiles {
		publicKeys, err := keyutil.PublicKeysFromFile(keyfile)
		if err != nil {
			return nil, err
		}
		allPublicKeys = append(allPublicKeys, publicKeys...)
	}

	tokenAuthenticator := serviceaccount.JWTTokenAuthenticator(issuers, allPublicKeys, apiAudiences, serviceaccount.NewValidator(serviceAccountGetter))
	return tokenAuthenticator, nil
}
