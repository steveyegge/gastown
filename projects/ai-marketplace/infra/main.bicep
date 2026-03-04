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

@description('Whether to deploy a new Cosmos DB account. Set to false when existingCosmosAccountName is provided.')
param deployCosmos bool = true

@description('Whether to deploy Azure Functions API backend')
param deployFunctions bool = true

@description('''
Name of an EXISTING Cosmos DB account to use instead of provisioning a new one.
When set, deployCosmos is ignored and this account is referenced directly.
Example: ai-marketplace-cosmos-p7a65r22uhdxo
''')
param existingCosmosAccountName string = ''

@description('Azure AD app registration client ID (from app registration)')
param azureAdClientId string = ''

@description('Azure AD / Entra ID tenant ID')
param azureAdTenantId string = ''

var useExistingCosmos = !empty(existingCosmosAccountName)
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

// ─── Cosmos DB — existing account reference ──────────────────────────────────
// Used when existingCosmosAccountName is set (e.g. ai-marketplace-cosmos-p7a65r22uhdxo)
resource existingCosmosAccount 'Microsoft.DocumentDB/databaseAccounts@2024-05-15' existing = if (useExistingCosmos) {
  name: existingCosmosAccountName
}

// ─── Cosmos DB — new account + containers (skipped when using existing) ──────
module cosmos 'modules/cosmos.bicep' = if (!useExistingCosmos && deployCosmos) {
  name: 'cosmos'
  params: {
    accountName: cosmosAccountName
    location: location
    databaseName: 'ai-marketplace'
  }
}

// ─── Cosmos DB — database + containers on the existing account ───────────────
// When using an existing account we still idempotently ensure all 10 containers exist.
module cosmosContainers 'modules/cosmos.bicep' = if (useExistingCosmos) {
  name: 'cosmosContainers'
  params: {
    accountName: existingCosmosAccountName
    location: location
    databaseName: 'ai-marketplace'
  }
}

// Resolve endpoint and key based on whether we're using existing or new account
var resolvedCosmosEndpoint = useExistingCosmos
  ? existingCosmosAccount.properties.documentEndpoint
  : (!useExistingCosmos && deployCosmos ? cosmos.outputs.endpoint : '')

var resolvedCosmosKey = useExistingCosmos
  ? existingCosmosAccount.listKeys().primaryMasterKey
  : (!useExistingCosmos && deployCosmos ? cosmos.outputs.primaryKey : '')

// Store the Cosmos primary key in Key Vault so Functions can reference it safely
resource cosmosKeySecret 'Microsoft.KeyVault/vaults/secrets@2023-07-01' = if (useExistingCosmos || deployCosmos) {
  name: '${keyVaultName}/cosmos-primary-key'
  properties: {
    value: resolvedCosmosKey
  }
  dependsOn: [keyVault]
}

// ─── Azure Functions API ─────────────────────────────────────────────────────
module functions 'modules/functions.bicep' = if (deployFunctions) {
  name: 'functions'
  params: {
    appName: functionsAppName
    location: location
    storageAccountName: storageAccountName
    appInsightsInstrumentationKey: appInsights.outputs.instrumentationKey
    cosmosEndpoint: resolvedCosmosEndpoint
    cosmosKey: (useExistingCosmos || deployCosmos)
      ? '@Microsoft.KeyVault(VaultName=${keyVaultName};SecretName=cosmos-primary-key)'
      : ''
    keyVaultName: keyVaultName
  }
  dependsOn: [cosmosKeySecret]
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
    apiBaseUrl: deployFunctions ? functions.outputs.defaultHostName : ''
    azureAdClientId: azureAdClientId
    azureAdTenantId: azureAdTenantId
  }
}

// Outputs
output appInsightsKey string = appInsights.outputs.instrumentationKey
output acrLoginServer string = acr.outputs.loginServer
output webAppUrl string = containerApp.outputs.appUrl
output cosmosEndpoint string = resolvedCosmosEndpoint
output cosmosAccountName string = useExistingCosmos ? existingCosmosAccountName : (!useExistingCosmos && deployCosmos ? cosmos.outputs.accountName : '')
output functionsUrl string = deployFunctions ? functions.outputs.defaultHostName : ''

