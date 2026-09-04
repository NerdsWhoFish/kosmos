# Architecture decisions

| # | Decision | In one line |
| --- | --- | --- |
| [0001](0001-publish-one-environment-module-per-kosmos-deployment.md) | Publish one environment module per Kosmos deployment | Kosmos publishes one cohesive GCP environment module that deployment roots consume from Spacelift. |
| [0002](0002-restrict-production-login-by-verified-google-email-domain.md) | Restrict production login by verified Google email domain | Allow only verified Google identities from three approved business domains. |
| [0003](0003-authorize-google-workspace-separately-from-sign-in.md) | Authorize Google Workspace separately from sign-in | Keep login scopes minimal and grant encrypted Gmail and Sheets access separately. |
