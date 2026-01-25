---
name: go-testing
description: >
  Go test patterns including table-driven tests, subtests, mocking, benchmarks,
  and coverage reporting. Best practices for reliable Go test suites.
allowed-tools: "Bash(go test:*),Bash(go:*),Read,Write"
version: "1.0.0"
author: "Gas Town"
license: "MIT"
---

# Go Testing - Test Patterns for Go Projects

Patterns and best practices for writing and running Go tests.

## Running Tests

### Basic Commands

```bash
# Run all tests in current directory
go test

# Run all tests recursively
go test ./...

# Run with verbose output
go test -v ./...

# Run specific test by name
go test -run TestUserAuth ./...

# Run with race detector
go test -race ./...
```

### Coverage

```bash
# Generate coverage report
go test -cover ./...

# Generate coverage profile
go test -coverprofile=coverage.out ./...

# View coverage in browser
go tool cover -html=coverage.out

# Get coverage percentage
go test -cover ./... | grep coverage
```

## Table-Driven Tests

The preferred pattern for testing multiple cases:

```go
func TestAdd(t *testing.T) {
    tests := []struct {
        name     string
        a, b     int
        expected int
    }{
        {"positive numbers", 2, 3, 5},
        {"with zero", 0, 5, 5},
        {"negative numbers", -1, -2, -3},
        {"mixed signs", -1, 5, 4},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := Add(tt.a, tt.b)
            if result != tt.expected {
                t.Errorf("Add(%d, %d) = %d; want %d",
                    tt.a, tt.b, result, tt.expected)
            }
        })
    }
}
```

## Subtests

Use `t.Run()` for organizing related tests:

```go
func TestUser(t *testing.T) {
    t.Run("Create", func(t *testing.T) {
        // Test user creation
    })

    t.Run("Update", func(t *testing.T) {
        // Test user update
    })

    t.Run("Delete", func(t *testing.T) {
        // Test user deletion
    })
}
```

Run specific subtest:
```bash
go test -run TestUser/Create ./...
```

## Test Fixtures

### Setup and Teardown

```go
func TestMain(m *testing.M) {
    // Setup before all tests
    setup()

    // Run tests
    code := m.Run()

    // Teardown after all tests
    teardown()

    os.Exit(code)
}
```

### Per-Test Setup

```go
func TestWithFixture(t *testing.T) {
    // Setup
    db := setupTestDB(t)
    t.Cleanup(func() {
        db.Close()
    })

    // Test code using db
}
```

### Testdata Directory

```go
func TestParseConfig(t *testing.T) {
    // Files in testdata/ are ignored by go build
    data, err := os.ReadFile("testdata/config.json")
    if err != nil {
        t.Fatal(err)
    }
    // Use data...
}
```

## Mocking

### Interface-Based Mocking

```go
// Define interface
type UserStore interface {
    Get(id string) (*User, error)
    Save(user *User) error
}

// Mock implementation
type MockUserStore struct {
    GetFunc  func(id string) (*User, error)
    SaveFunc func(user *User) error
}

func (m *MockUserStore) Get(id string) (*User, error) {
    return m.GetFunc(id)
}

func (m *MockUserStore) Save(user *User) error {
    return m.SaveFunc(user)
}

// Use in test
func TestService(t *testing.T) {
    mock := &MockUserStore{
        GetFunc: func(id string) (*User, error) {
            return &User{ID: id, Name: "Test"}, nil
        },
    }

    svc := NewService(mock)
    // Test svc...
}
```

## Benchmark Tests

```go
func BenchmarkFibonacci(b *testing.B) {
    for i := 0; i < b.N; i++ {
        Fibonacci(20)
    }
}

// With setup
func BenchmarkProcess(b *testing.B) {
    data := generateTestData()
    b.ResetTimer() // Don't count setup time

    for i := 0; i < b.N; i++ {
        Process(data)
    }
}
```

Run benchmarks:
```bash
go test -bench=. ./...
go test -bench=BenchmarkFibonacci -benchmem ./...
```

## Parallel Tests

```go
func TestParallel(t *testing.T) {
    t.Parallel() // Mark test as parallel-safe

    tests := []struct {
        name string
        // ...
    }{
        // test cases
    }

    for _, tt := range tests {
        tt := tt // Capture range variable
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel() // Each subtest runs in parallel
            // Test code
        })
    }
}
```

## Helper Functions

```go
func setupTestServer(t *testing.T) *httptest.Server {
    t.Helper() // Marks as helper for better error reporting

    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    })

    server := httptest.NewServer(handler)
    t.Cleanup(server.Close)

    return server
}
```

## Error Assertions

```go
func TestErrors(t *testing.T) {
    _, err := DoSomething()

    // Check error occurred
    if err == nil {
        t.Fatal("expected error, got nil")
    }

    // Check specific error
    if !errors.Is(err, ErrNotFound) {
        t.Errorf("got %v; want %v", err, ErrNotFound)
    }

    // Check error message contains text
    if !strings.Contains(err.Error(), "not found") {
        t.Errorf("error %q should contain 'not found'", err)
    }
}
```

## Quick Reference

| Command | Description |
|---------|-------------|
| `go test` | Run tests in current package |
| `go test ./...` | Run all tests recursively |
| `go test -v` | Verbose output |
| `go test -run Name` | Run specific test |
| `go test -cover` | Show coverage |
| `go test -race` | Enable race detector |
| `go test -bench=.` | Run benchmarks |
| `go test -short` | Skip long tests |
| `go test -count=1` | Disable test caching |
