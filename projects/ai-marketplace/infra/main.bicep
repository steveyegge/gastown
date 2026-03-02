targetScope = 'resourceGroup'

@description('The name prefix for all resources')
param appName string = 'ai-marketplace'

@description('Azure region for all resources')
param location string = resourceGroup().location

@description('Deployment environment')
@allowed(['dev', 'staging', 'prod'])
param environment string = 'dev'

@description('Azure AD tenant ID for Entra ID app registration')
param tenantId string = subscription().tenantId

@description('Container image tag to deploy')
param imageTag string = 'latest'

@description('Whether to deploy Cosmos DB (skip if region has capacity issues)')
param deployCosmos bool = false

@description('Whether to deploy Azure Functions API backend')
param deployFunctions bool = false

var suffix = uniqueString(resourceGroup().id)
var shortSuffix = substring(suffix, 0, 8)
var cosmosAccountName = '${appName}-cosmos-${suffix}'
var functionsAppName = '${appName}-api-${environment}'
var storageAccountName = 'aimktstore${shortSuffix}'
var appInsightsName = '${appName}-insights-${environment}'
var keyVaultName = 'aimkt-kv-${shortSuffix}'
var acrName = 'aimktacr${shortSuffix}'
var containerAppEnvName = '${appName}-env-${environment}'
var containerAppName = '${appName}-web-${environment}'

// Application Insights
module appInsights 'modules/appinsights.bicep' = {
  name: 'appInsights'
  params: {
    name: appInsightsName
    location: location
  }
}

// Key Vault
module keyVault 'modules/keyvault.bicep' = {
  name: 'keyVault'
  params: {
    name: keyVaultName
    location: location
    tenantId: tenantId
  }
}

// Container Registry
module acr 'modules/containerregistry.bicep' = {
  name: 'containerRegistry'
  params: {
    name: acrName
    location: location
  }
}

// Container App (web frontend)
module containerApp 'modules/containerapp.bicep' = {
  name: 'containerApp'
  params: {
    environmentName: containerAppEnvName
    appName: containerAppName
    location: location
    containerImage: 'mcr.microsoft.com/k8se/quickstart:latest'
    acrLoginServer: acr.outputs.loginServer
    acrName: acr.outputs.name
    appInsightsConnectionString: appInsights.outputs.connectionString
    apiBaseUrl: ''
  }
}

// Outputs
output appInsightsKey string = appInsights.outputs.instrumentationKey
output acrLoginServer string = acr.outputs.loginServer
output webAppUrl string = containerApp.outputs.appUrl
