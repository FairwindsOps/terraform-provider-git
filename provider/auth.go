package provider

import (
	"errors"

	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"golang.org/x/crypto/ssh/knownhosts"
)

func authSchema() *schema.Schema {
	return &schema.Schema{
		Description: "An authentication block for connecting to the git provider.",
		Type:        schema.TypeList,
		Optional:    true,
		MaxItems:    1,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"bearer": {
					Type:     schema.TypeList,
					Optional: true,
					MaxItems: 1,
					Elem: &schema.Resource{
						Schema: map[string]*schema.Schema{
							"token": {
								Type:     schema.TypeString,
								Required: true,
							},
						},
					},
				},
				"basic": {
					Type:     schema.TypeList,
					Optional: true,
					MaxItems: 1,
					Elem: &schema.Resource{
						Schema: map[string]*schema.Schema{
							"username": {
								Type:     schema.TypeString,
								Optional: true,
								Default:  "git",
							},
							"password": {
								Type:     schema.TypeString,
								Required: true,
							},
						},
					},
				},
				"ssh_key": {
					Type:     schema.TypeList,
					Optional: true,
					MaxItems: 1,
					Elem: &schema.Resource{
						Schema: map[string]*schema.Schema{
							"username": {
								Type:     schema.TypeString,
								Optional: true,
								Default:  "git",
							},
							"private_key_pem": {
								Type:     schema.TypeString,
								Optional: true,
							},
							"private_key_path": {
								Type:     schema.TypeString,
								Optional: true,
							},
							"password": {
								Type:     schema.TypeString,
								Optional: true,
								Default:  "",
							},
							"known_hosts": {
								Type:     schema.TypeList,
								Optional: true,
								Elem: &schema.Schema{
									Type: schema.TypeString,
								},
							},
						},
					},
				},
			},
		},
	}
}

func getAuth(d *schema.ResourceData) (transport.AuthMethod, error) {
	authData := getMapItem(d.Get("auth"))
	if authData == nil {
		return nil, nil
	}

	if sshKey := getMapItem(authData["ssh_key"]); sshKey != nil {
		username := sshKey["username"].(string)
		password := sshKey["password"].(string)
		knownHosts := sshKey["known_hosts"].([]interface{})

		var publicKeys *ssh.PublicKeys
		var err error
		if sshKey["private_key_pem"] != nil {
			privateKeyPem := sshKey["private_key_pem"].(string)
			publicKeys, err = ssh.NewPublicKeys(username, []byte(privateKeyPem), password)
			if err != nil {
				return nil, err
			}
		} else if sshKey["private_key_path"] != nil {
			privateKeyPath := sshKey["private_key_path"].(string)
			publicKeys, err = ssh.NewPublicKeysFromFile(username, privateKeyPath, password)
			if err != nil {
				return nil, err
			}
		}

		if len(knownHosts) > 0 {
			knownHostStrings := make([]string, len(knownHosts))
			for i, knownHost := range knownHosts {
				knownHosts[i] = knownHost.(string)
			}
			callback, err := knownhosts.New(knownHostStrings...)
			if err != nil {
				return nil, err
			}
			publicKeys.HostKeyCallback = callback
		}

		return publicKeys, nil
	}

	if basic := getMapItem(authData["basic"]); basic != nil {
		username := basic["username"].(string)
		password := basic["password"].(string)

		return &http.BasicAuth{
			Username: username,
			Password: password,
		}, nil
	}

	if bearer := getMapItem(authData["bearer"]); bearer != nil {
		token := bearer["token"].(string)

		return &http.TokenAuth{
			Token: token,
		}, nil
	}

	return nil, errors.New("unknown auth method")
}
