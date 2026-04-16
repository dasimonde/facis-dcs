{
  "realm": "gaia-x",
  "displayName": "GAIA-X",
  "enabled": true,
  "sslRequired": "none",
  "registrationAllowed": false,
  "loginWithEmailAllowed": true,
  "duplicateEmailsAllowed": false,
  "resetPasswordAllowed": false,
  "editUsernameAllowed": false,
  "bruteForceProtected": false,
  "roles": {
    "realm": [
      {
        "name": "uma_authorization",
        "description": "${role_uma_authorization}"
      },
      {
        "name": "offline_access",
        "description": "${role_offline-access}"
      },
      {
        "name": "default-roles-gaia-x",
        "description": "${role_default-roles}"
      }
    ],
    "client": {
      "federated-catalogue": [
        {
          "name": "Ro-SD-A"
        },
        {
          "name": "Ro-PA-A"
        },
        {
          "name": "uma_protection"
        },
        {
          "name": "Ro-MU-CA",
          "composite": true,
          "composites": {
            "client": {
              "federated-catalogue": [
                "Ro-SD-A",
                "Ro-PA-A",
                "Ro-MU-A"
              ]
            }
          }
        },
        {
          "name": "Ro-MU-A"
        }
      ],
      "dcs-client": [
        {
          "name": "Template Creator",
          "clientRole": true
        },
        {
          "name": "Template Reviewer",
          "clientRole": true
        },
        {
          "name": "Template Approver",
          "clientRole": true
        },
        {
          "name": "Template Manager",
          "clientRole": true
        },
        {
          "name": "Contract Creator",
          "clientRole": true
        },
        {
          "name": "Contract Negotiator",
          "clientRole": true
        },
        {
          "name": "Contract Reviewer",
          "clientRole": true
        },
        {
          "name": "Contract Approver",
          "clientRole": true
        },
        {
          "name": "Contract Manager",
          "clientRole": true
        },
        {
          "name": "Contract Signer",
          "clientRole": true
        },
        {
          "name": "Contract Observer",
          "clientRole": true
        },
        {
          "name": "Sys. Contract Creator",
          "clientRole": true
        },
        {
          "name": "Sys. Contract Reviewer",
          "clientRole": true
        },
        {
          "name": "Sys. Contract Approver",
          "clientRole": true
        },
        {
          "name": "Sys. Contract Manager",
          "clientRole": true
        },
        {
          "name": "Sys. Contract Signer",
          "clientRole": true
        },
        {
          "name": "Archive Manager",
          "clientRole": true
        },
        {
          "name": "Auditor",
          "clientRole": true
        },
        {
          "name": "Compliance Officer",
          "clientRole": true
        }
      ]
    }
  },
  "users": [
    {
      "username": "service-account-federated-catalogue",
      "enabled": true,
      "serviceAccountClientId": "federated-catalogue",
      "realmRoles": [
        "default-roles-gaia-x"
      ],
      "clientRoles": {
        "federated-catalogue": [
          "uma_protection"
        ],
        "realm-management": [
          "manage-users",
          "view-users",
          "view-clients"
        ]
      }
    },
    {
      "username": "test",
      "enabled": true,
      "email": "test@gaia-x.local",
      "emailVerified": true,
      "firstName": "Test",
      "lastName": "User",
      "credentials": [
        {
          "type": "password",
          "value": "test",
          "temporary": false
        }
      ],
      "realmRoles": [
        "default-roles-gaia-x"
      ],
      "clientRoles": {
        "federated-catalogue": [
          "Ro-MU-CA",
          "Ro-SD-A",
          "Ro-PA-A",
          "Ro-MU-A"
        ],
        "dcs-client": [
          "Template Creator",
          "Template Reviewer",
          "Template Approver",
          "Template Manager",
          "Contract Creator",
          "Contract Negotiator",
          "Contract Reviewer",
          "Contract Approver",
          "Contract Manager",
          "Contract Signer",
          "Contract Observer",
          "Sys. Contract Creator",
          "Sys. Contract Reviewer",
          "Sys. Contract Approver",
          "Sys. Contract Manager",
          "Sys. Contract Signer",
          "Archive Manager",
          "Auditor",
          "Compliance Officer"
        ]
      }
    }
  ],
  "clients": [
    {
      "clientId": "federated-catalogue",
      "enabled": true,
      "clientAuthenticatorType": "client-secret",
      "secret": "federated-catalogue-secret",
      "redirectUris": {{ toJson .Values.realm.dcsClient.redirectUris }},
      "webOrigins": {{ toJson .Values.realm.dcsClient.webOrigins }},
      "standardFlowEnabled": true,
      "directAccessGrantsEnabled": true,
      "serviceAccountsEnabled": true,
      "authorizationServicesEnabled": true,
      "publicClient": false,
      "protocol": "openid-connect",
      "fullScopeAllowed": true,
      "protocolMappers": [
        {
          "name": "Client Host",
          "protocol": "openid-connect",
          "protocolMapper": "oidc-usersessionmodel-note-mapper",
          "config": {
            "user.session.note": "clientHost",
            "userinfo.token.claim": "true",
            "id.token.claim": "true",
            "access.token.claim": "true",
            "claim.name": "clientHost",
            "jsonType.label": "String"
          }
        },
        {
          "name": "Client ID",
          "protocol": "openid-connect",
          "protocolMapper": "oidc-usersessionmodel-note-mapper",
          "config": {
            "user.session.note": "clientId",
            "userinfo.token.claim": "true",
            "id.token.claim": "true",
            "access.token.claim": "true",
            "claim.name": "clientId",
            "jsonType.label": "String"
          }
        }
      ],
      "defaultClientScopes": [
        "openid",
        "profile",
        "email",
        "web-origins",
        "gaia-x",
        "roles"
      ],
      "optionalClientScopes": [
        "participant",
        "self-description",
        "offline_access"
      ],
      "authorizationSettings": {
        "allowRemoteResourceManagement": false,
        "policyEnforcementMode": "ENFORCING",
        "resources": [
          {
            "name": "Default Resource",
            "type": "urn:federated-catalogue:resources:default",
            "uris": [
              "/*"
            ]
          }
        ],
        "policies": [
          {
            "name": "Default Permission",
            "type": "resource",
            "logic": "POSITIVE",
            "decisionStrategy": "UNANIMOUS",
            "config": {
              "defaultResourceType": "urn:federated-catalogue:resources:default",
              "applyPolicies": "[]"
            }
          }
        ]
      }
    },
    {
      "clientId": "dcs-client",
      "enabled": true,
      "publicClient": true,
      "standardFlowEnabled": true,
      "directAccessGrantsEnabled": true,
      "serviceAccountsEnabled": false,
      "protocolMappers": [
        {
          "name": "sub",
          "protocol": "openid-connect",
          "protocolMapper": "oidc-sub-mapper",
          "consentRequired": false,
          "config": {
            "access.token.claim": "true",
            "id.token.claim": "true",
            "userinfo.token.claim": "true"
          }
        }
      ],
      "redirectUris": {{ toJson .Values.realm.dcsClient.redirectUris }},
      "webOrigins": {{ toJson .Values.realm.dcsClient.webOrigins }},
      "protocol": "openid-connect",
      "frontchannelLogout": true,
      "attributes": {
        "post.logout.redirect.uris": "+"
      },
      "defaultClientScopes": [
        "web-origins",
        "profile",
        "roles",
        "email",
        "gaia-x"
      ],
      "optionalClientScopes": [
        "offline_access"
      ]
    }
  ],
  "clientScopes": [
    {
      "name": "openid",
      "protocol": "openid-connect",
      "attributes": {
        "include.in.token.scope": "true",
        "display.on.consent.screen": "true"
      },
      "protocolMappers": []
    },
    {
      "name": "profile",
      "protocol": "openid-connect",
      "attributes": {
        "include.in.token.scope": "true",
        "display.on.consent.screen": "true"
      },
      "protocolMappers": [
        {
          "name": "username",
          "protocol": "openid-connect",
          "protocolMapper": "oidc-usermodel-property-mapper",
          "config": {
            "user.attribute": "username",
            "id.token.claim": "true",
            "access.token.claim": "true",
            "claim.name": "preferred_username",
            "userinfo.token.claim": "true"
          }
        },
        {
          "name": "full name",
          "protocol": "openid-connect",
          "protocolMapper": "oidc-full-name-mapper",
          "config": {
            "id.token.claim": "true",
            "access.token.claim": "true",
            "userinfo.token.claim": "true"
          }
        }
      ]
    },
    {
      "name": "email",
      "protocol": "openid-connect",
      "attributes": {
        "include.in.token.scope": "true",
        "display.on.consent.screen": "true"
      },
      "protocolMappers": [
        {
          "name": "email",
          "protocol": "openid-connect",
          "protocolMapper": "oidc-usermodel-property-mapper",
          "config": {
            "user.attribute": "email",
            "id.token.claim": "true",
            "access.token.claim": "true",
            "claim.name": "email",
            "userinfo.token.claim": "true"
          }
        },
        {
          "name": "email verified",
          "protocol": "openid-connect",
          "protocolMapper": "oidc-usermodel-property-mapper",
          "config": {
            "user.attribute": "emailVerified",
            "id.token.claim": "true",
            "access.token.claim": "true",
            "claim.name": "email_verified",
            "userinfo.token.claim": "true"
          }
        }
      ]
    },
    {
      "name": "roles",
      "protocol": "openid-connect",
      "attributes": {
        "include.in.token.scope": "false",
        "display.on.consent.screen": "true"
      },
      "protocolMappers": [
        {
          "name": "realm roles",
          "protocol": "openid-connect",
          "protocolMapper": "oidc-usermodel-realm-role-mapper",
          "config": {
            "user.attribute": "foo",
            "access.token.claim": "true",
            "claim.name": "realm_access.roles",
            "jsonType.label": "String",
            "multivalued": "true"
          }
        },
        {
          "name": "client roles",
          "protocol": "openid-connect",
          "protocolMapper": "oidc-usermodel-client-role-mapper",
          "config": {
            "user.attribute": "foo",
            "access.token.claim": "true",
            "claim.name": "resource_access.${client_id}.roles",
            "jsonType.label": "String",
            "multivalued": "true"
          }
        }
      ]
    },
    {
      "name": "web-origins",
      "protocol": "openid-connect",
      "attributes": {
        "include.in.token.scope": "false",
        "display.on.consent.screen": "false"
      },
      "protocolMappers": [
        {
          "name": "allowed web origins",
          "protocol": "openid-connect",
          "protocolMapper": "oidc-allowed-origins-mapper",
          "config": {}
        }
      ]
    },
    {
      "name": "gaia-x",
      "protocol": "openid-connect",
      "attributes": {
        "include.in.token.scope": "true"
      },
      "protocolMappers": [
        {
          "name": "realm roles",
          "protocol": "openid-connect",
          "protocolMapper": "oidc-usermodel-realm-role-mapper",
          "config": {
            "access.token.claim": "true",
            "claim.name": "realm_access.roles",
            "jsonType.label": "String",
            "multivalued": "true"
          }
        },
        {
          "name": "client roles",
          "protocol": "openid-connect",
          "protocolMapper": "oidc-usermodel-client-role-mapper",
          "config": {
            "access.token.claim": "true",
            "claim.name": "resource_access.${client_id}.roles",
            "jsonType.label": "String",
            "multivalued": "true"
          }
        },
        {
          "name": "participant_id",
          "protocol": "openid-connect",
          "protocolMapper": "oidc-usermodel-attribute-mapper",
          "config": {
            "user.attribute": "participantId",
            "id.token.claim": "true",
            "access.token.claim": "true",
            "claim.name": "participant_id",
            "userinfo.token.claim": "true"
          }
        }
      ]
    },
    {
      "name": "participant",
      "protocol": "openid-connect",
      "attributes": {
        "include.in.token.scope": "true"
      }
    },
    {
      "name": "self-description",
      "protocol": "openid-connect",
      "attributes": {
        "include.in.token.scope": "true"
      }
    }
  ]
}
