@description('Cosmos DB account name')
param accountName string

@description('Azure region')
param location string

@description('Database name')
param databaseName string = 'ai-marketplace'

resource cosmosAccount 'Microsoft.DocumentDB/databaseAccounts@2024-05-15' = {
  name: accountName
  location: location
  kind: 'GlobalDocumentDB'
  properties: {
    databaseAccountOfferType: 'Standard'
    consistencyPolicy: {
      defaultConsistencyLevel: 'Session'
    }
    locations: [
      {
        locationName: location
        failoverPriority: 0
        isZoneRedundant: false
      }
    ]
    capabilities: [
      { name: 'EnableServerless' }
    ]
    backupPolicy: {
      type: 'Continuous'
      continuousModeProperties: { tier: 'Continuous7Days' }
    }
  }
}

resource database 'Microsoft.DocumentDB/databaseAccounts/sqlDatabases@2024-05-15' = {
  parent: cosmosAccount
  name: databaseName
  properties: {
    resource: { id: databaseName }
  }
}

// ─── Containers with partition keys ─────────────────────────────────────────

resource assetsContainer 'Microsoft.DocumentDB/databaseAccounts/sqlDatabases/containers@2024-05-15' = {
  parent: database
  name: 'assets'
  properties: {
    resource: {
      id: 'assets'
      partitionKey: {
        paths: ['/tenantId', '/type']
        kind: 'MultiHash'
        version: 2
      }
      indexingPolicy: {
        indexingMode: 'consistent'
        includedPaths: [{ path: '/*' }]
        excludedPaths: [{ path: '/"_etag"/?' }]
        compositeIndexes: [
          [
            { path: '/deploymentCount', order: 'descending' }
            { path: '/rating', order: 'descending' }
          ]
        ]
      }
    }
  }
}

resource publishersContainer 'Microsoft.DocumentDB/databaseAccounts/sqlDatabases/containers@2024-05-15' = {
  parent: database
  name: 'publishers'
  properties: {
    resource: {
      id: 'publishers'
      partitionKey: { paths: ['/publisherId'], kind: 'Hash' }
    }
  }
}

resource submissionsContainer 'Microsoft.DocumentDB/databaseAccounts/sqlDatabases/containers@2024-05-15' = {
  parent: database
  name: 'submissions'
  properties: {
    resource: {
      id: 'submissions'
      partitionKey: { paths: ['/tenantId'], kind: 'Hash' }
      defaultTtl: -1
    }
  }
}

resource workflowsContainer 'Microsoft.DocumentDB/databaseAccounts/sqlDatabases/containers@2024-05-15' = {
  parent: database
  name: 'workflows'
  properties: {
    resource: {
      id: 'workflows'
      partitionKey: { paths: ['/tenantId'], kind: 'Hash' }
    }
  }
}

resource auditLogContainer 'Microsoft.DocumentDB/databaseAccounts/sqlDatabases/containers@2024-05-15' = {
  parent: database
  name: 'audit-log'
  properties: {
    resource: {
      id: 'audit-log'
      partitionKey: { paths: ['/date'], kind: 'Hash' }
      defaultTtl: 7776000  // 90 days TTL for old audit records
    }
  }
}

output endpoint string = cosmosAccount.properties.documentEndpoint
output primaryKey string = cosmosAccount.listKeys().primaryMasterKey
output accountName string = cosmosAccount.name
