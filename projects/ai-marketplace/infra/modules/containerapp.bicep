@description('Name of the Container Apps Environment')
param environmentName string

@description('Azure AD app registration client ID')
param azureAdClientId string = ''

@description('Azure AD / Entra ID tenant ID')
param azureAdTenantId string = ''

@description('Name of the Container App')
param appName string

@description('Azure region')
param location string

@description('Container image (e.g. myacr.azurecr.io/web:latest)')
param containerImage string = 'mcr.microsoft.com/k8se/quickstart:latest'

@description('ACR login server')
param acrLoginServer string

@description('ACR name (for pull credentials)')
param acrName string

@description('App Insights connection string')
param appInsightsConnectionString string

@description('API base URL for the frontend')
param apiBaseUrl string = ''

// ── Log Analytics workspace for the environment ───────────────────────────
resource logAnalytics 'Microsoft.OperationalInsights/workspaces@2022-10-01' = {
  name: '${environmentName}-logs'
  location: location
  properties: {
    sku: { name: 'PerGB2018' }
    retentionInDays: 30
  }
}

// ── Container Apps Environment ────────────────────────────────────────────
resource env 'Microsoft.App/managedEnvironments@2024-03-01' = {
  name: environmentName
  location: location
  properties: {
    appLogsConfiguration: {
      destination: 'log-analytics'
      logAnalyticsConfiguration: {
        customerId: logAnalytics.properties.customerId
        sharedKey: logAnalytics.listKeys().primarySharedKey
      }
    }
  }
}

// ── ACR reference for pull secret ────────────────────────────────────────
resource acr 'Microsoft.ContainerRegistry/registries@2023-07-01' existing = {
  name: acrName
}

// ── Container App ─────────────────────────────────────────────────────────
resource app 'Microsoft.App/containerApps@2024-03-01' = {
  name: appName
  location: location
  tags: {
    'azd-service-name': 'web'
  }
  properties: {
    managedEnvironmentId: env.id
    configuration: {
      ingress: {
        external: true
        targetPort: 3000
        transport: 'auto'
      }
      registries: [
        {
          server: acrLoginServer
          username: acr.listCredentials().username
          passwordSecretRef: 'acr-password'
        }
      ]
      secrets: [
        {
          name: 'acr-password'
          value: acr.listCredentials().passwords[0].value
        }
      ]
    }
    template: {
      containers: [
        {
          name: appName
          image: containerImage
          resources: {
            cpu: json('0.5')
            memory: '1Gi'
          }
          env: [
            {
              name: 'NEXT_PUBLIC_API_BASE_URL'
              value: apiBaseUrl
            }
            {
              name: 'APPLICATIONINSIGHTS_CONNECTION_STRING'
              value: appInsightsConnectionString
            }
            {
              name: 'NODE_ENV'
              value: 'production'
            }
            {
              name: 'AZURE_AD_CLIENT_ID'
              value: azureAdClientId
            }
            {
              name: 'AZURE_AD_TENANT_ID'
              value: azureAdTenantId
            }
          ]
        }
      ]
      scale: {
        minReplicas: 0
        maxReplicas: 3
        rules: [
          {
            name: 'http-scaling'
            http: {
              metadata: {
                concurrentRequests: '10'
              }
            }
          }
        ]
      }
    }
  }
}

output fqdn string = app.properties.configuration.ingress.fqdn
output appUrl string = 'https://${app.properties.configuration.ingress.fqdn}'
