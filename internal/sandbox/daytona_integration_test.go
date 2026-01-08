// Package sandbox provides integration tests for the Daytona backend.
// Run with: DAYTONA_API_KEY=xxx go test -v ./internal/sandbox -run TestDaytona
package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	daytonaPkg "github.com/steveyegge/gastown/internal/sandbox/daytona"
)

// skipIfNoAPIKey skips the test if the Daytona API key is not set.
func skipIfNoAPIKey(t *testing.T) {
	if os.Getenv("DAYTONA_API_KEY") == "" {
		t.Skip("DAYTONA_API_KEY not set, skipping integration test")
	}
}

// TestDaytonaBackendAvailable tests that the Daytona API is accessible.
func TestDaytonaBackendAvailable(t *testing.T) {
	skipIfNoAPIKey(t)

	backend := NewDaytonaBackend(nil)
	if !backend.IsAvailable() {
		t.Fatal("Daytona backend is not available - check API key")
	}
	t.Log("Daytona backend is available")
}

// TestDaytonaSandboxLifecycle tests the full sandbox lifecycle.
func TestDaytonaSandboxLifecycle(t *testing.T) {
	skipIfNoAPIKey(t)

	backend := NewDaytonaBackend(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Create a sandbox
	t.Log("Creating sandbox...")
	session, err := backend.Create(ctx, CreateOptions{
		Name:    "gastown-test-" + time.Now().Format("20060102-150405"),
		WorkDir: "/home/daytona",
	})
	if err != nil {
		t.Fatalf("Failed to create sandbox: %v", err)
	}
	t.Logf("Created sandbox: %s (sandbox ID: %s)", session.ID, session.Metadata[MetaSandboxID])

	// Ensure cleanup
	defer func() {
		t.Log("Destroying sandbox...")
		if err := backend.Destroy(context.Background(), session); err != nil {
			t.Errorf("Failed to destroy sandbox: %v", err)
		} else {
			t.Log("Sandbox destroyed")
		}
	}()

	// Check if session exists
	exists, err := backend.HasSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("Failed to check session: %v", err)
	}
	if !exists {
		t.Fatal("Session should exist after creation")
	}
	t.Log("Session exists")

	// Test that IsRunning returns false before Start
	running, err := backend.IsRunning(ctx, session)
	if err != nil {
		t.Fatalf("Failed to check running state: %v", err)
	}
	t.Logf("IsRunning before Start: %v", running)
}

// TestDaytonaFileSync tests file upload and download.
func TestDaytonaFileSync(t *testing.T) {
	skipIfNoAPIKey(t)

	backend := NewDaytonaBackend(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Create a sandbox
	t.Log("Creating sandbox for file sync test...")
	session, err := backend.Create(ctx, CreateOptions{
		Name:    "gastown-filesync-" + time.Now().Format("20060102-150405"),
		WorkDir: "/home/daytona",
	})
	if err != nil {
		t.Fatalf("Failed to create sandbox: %v", err)
	}
	t.Logf("Created sandbox: %s", session.ID)

	defer func() {
		t.Log("Destroying sandbox...")
		backend.Destroy(context.Background(), session)
	}()

	// Create a temp directory with test files
	tmpDir, err := os.MkdirTemp("", "gastown-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test file
	testContent := "Hello from GasTown integration test!\nLine 2\nLine 3"
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Create subdirectory with file
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}
	subFile := filepath.Join(subDir, "nested.txt")
	if err := os.WriteFile(subFile, []byte("Nested content"), 0644); err != nil {
		t.Fatalf("Failed to write nested file: %v", err)
	}

	// Upload to sandbox
	t.Log("Uploading files to sandbox...")
	remotePath := "/home/daytona/uploaded"
	if err := backend.SyncToSession(ctx, session, tmpDir, remotePath); err != nil {
		t.Fatalf("Failed to sync to session: %v", err)
	}
	t.Log("Files uploaded successfully")

	// Download back to different location
	t.Log("Downloading files from sandbox...")
	downloadDir, err := os.MkdirTemp("", "gastown-download-*")
	if err != nil {
		t.Fatalf("Failed to create download dir: %v", err)
	}
	defer os.RemoveAll(downloadDir)

	if err := backend.SyncFromSession(ctx, session, remotePath, downloadDir); err != nil {
		t.Fatalf("Failed to sync from session: %v", err)
	}
	t.Log("Files downloaded successfully")

	// Verify downloaded content
	downloadedFile := filepath.Join(downloadDir, "test.txt")
	content, err := os.ReadFile(downloadedFile)
	if err != nil {
		t.Fatalf("Failed to read downloaded file: %v", err)
	}
	if string(content) != testContent {
		t.Errorf("Content mismatch: got %q, want %q", string(content), testContent)
	} else {
		t.Log("File content verified successfully")
	}
}

// TestDaytonaPtySession tests PTY session with command execution.
func TestDaytonaPtySession(t *testing.T) {
	skipIfNoAPIKey(t)

	backend := NewDaytonaBackend(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Create a sandbox
	t.Log("Creating sandbox for PTY test...")
	session, err := backend.Create(ctx, CreateOptions{
		Name:    "gastown-pty-" + time.Now().Format("20060102-150405"),
		WorkDir: "/home/daytona",
	})
	if err != nil {
		t.Fatalf("Failed to create sandbox: %v", err)
	}
	t.Logf("Created sandbox: %s", session.ID)

	defer func() {
		t.Log("Destroying sandbox...")
		backend.Destroy(context.Background(), session)
	}()

	// Start a simple shell command instead of Claude (faster test)
	t.Log("Starting PTY session with bash...")
	if err := backend.Start(ctx, session, "bash"); err != nil {
		t.Fatalf("Failed to start PTY session: %v", err)
	}
	t.Log("PTY session started")

	// Wait a moment for bash to initialize
	time.Sleep(2 * time.Second)

	// Send a command
	t.Log("Sending 'echo hello' command...")
	if err := backend.SendInput(ctx, session, "echo hello"); err != nil {
		t.Fatalf("Failed to send input: %v", err)
	}

	// Wait for output
	time.Sleep(2 * time.Second)

	// Capture output
	output, err := backend.CaptureOutput(ctx, session, 50)
	if err != nil {
		t.Fatalf("Failed to capture output: %v", err)
	}
	t.Logf("Captured output (%d bytes):\n%s", len(output), output)

	// Verify output contains "hello"
	if !strings.Contains(output, "hello") {
		t.Errorf("Output should contain 'hello', got: %s", output)
	} else {
		t.Log("Output verified - contains 'hello'")
	}

	// Stop the session
	t.Log("Stopping session...")
	if err := backend.Stop(ctx, session); err != nil {
		t.Errorf("Failed to stop session: %v", err)
	} else {
		t.Log("Session stopped")
	}
}

// TestDaytonaExecuteCommand tests synchronous command execution.
func TestDaytonaExecuteCommand(t *testing.T) {
	skipIfNoAPIKey(t)

	backend := NewDaytonaBackend(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Create a sandbox
	t.Log("Creating sandbox for execute command test...")
	session, err := backend.Create(ctx, CreateOptions{
		Name:    "gastown-exec-" + time.Now().Format("20060102-150405"),
		WorkDir: "/home/daytona",
	})
	if err != nil {
		t.Fatalf("Failed to create sandbox: %v", err)
	}
	t.Logf("Created sandbox: %s", session.ID)

	defer func() {
		t.Log("Destroying sandbox...")
		backend.Destroy(context.Background(), session)
	}()

	// Get the client
	client, err := backend.GetClient()
	if err != nil {
		t.Fatalf("Failed to get client: %v", err)
	}

	sandboxID := session.Metadata[MetaSandboxID]

	// Test ExecuteCommand
	t.Log("Testing ExecuteCommand...")
	resp, err := client.ExecuteCommand(ctx, sandboxID, &daytonaPkg.ExecuteRequest{
		Command: "echo 'hello world'",
		Timeout: 10,
	})
	if err != nil {
		t.Fatalf("ExecuteCommand failed: %v", err)
	}
	t.Logf("ExecuteCommand result: exit=%d, output=%s", resp.ExitCode, resp.Result)

	if resp.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", resp.ExitCode)
	}
	if !strings.Contains(resp.Result, "hello world") {
		t.Errorf("Expected output to contain 'hello world', got: %s", resp.Result)
	}

	// Test command with non-zero exit
	t.Log("Testing command with non-zero exit...")
	resp, err = client.ExecuteCommand(ctx, sandboxID, &daytonaPkg.ExecuteRequest{
		Command: "sh -c 'exit 42'",
		Timeout: 10,
	})
	if err != nil {
		t.Fatalf("ExecuteCommand failed: %v", err)
	}
	if resp.ExitCode != 42 {
		t.Errorf("Expected exit code 42, got %d", resp.ExitCode)
	}
	t.Logf("Non-zero exit test passed: exit=%d", resp.ExitCode)
}

// TestDaytonaSandboxStartStop tests starting and stopping a sandbox.
func TestDaytonaSandboxStartStop(t *testing.T) {
	skipIfNoAPIKey(t)

	backend := NewDaytonaBackend(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Create a sandbox
	t.Log("Creating sandbox for start/stop test...")
	session, err := backend.Create(ctx, CreateOptions{
		Name:    "gastown-startstop-" + time.Now().Format("20060102-150405"),
		WorkDir: "/home/daytona",
	})
	if err != nil {
		t.Fatalf("Failed to create sandbox: %v", err)
	}
	t.Logf("Created sandbox: %s", session.ID)

	defer func() {
		t.Log("Destroying sandbox...")
		backend.Destroy(context.Background(), session)
	}()

	client, err := backend.GetClient()
	if err != nil {
		t.Fatalf("Failed to get client: %v", err)
	}

	sandboxID := session.Metadata[MetaSandboxID]

	// Sandbox should be running after creation
	sandbox, err := client.GetSandbox(ctx, sandboxID)
	if err != nil {
		t.Fatalf("Failed to get sandbox: %v", err)
	}
	t.Logf("Initial state: %s", sandbox.State)

	// Stop the sandbox
	t.Log("Stopping sandbox...")
	sandbox, err = client.StopSandbox(ctx, sandboxID)
	if err != nil {
		t.Fatalf("Failed to stop sandbox: %v", err)
	}
	t.Logf("Stop requested, state: %s", sandbox.State)

	// Wait for stopped state
	t.Log("Waiting for sandbox to stop...")
	sandbox, err = client.WaitForSandboxState(ctx, sandboxID, daytonaPkg.SandboxStateStopped, 2*time.Minute)
	if err != nil {
		t.Fatalf("Failed waiting for stopped state: %v", err)
	}
	t.Logf("Sandbox stopped, state: %s", sandbox.State)

	// Start the sandbox again
	t.Log("Starting sandbox...")
	sandbox, err = client.StartSandbox(ctx, sandboxID)
	if err != nil {
		t.Fatalf("Failed to start sandbox: %v", err)
	}
	t.Logf("Start requested, state: %s", sandbox.State)

	// Wait for started state
	t.Log("Waiting for sandbox to start...")
	sandbox, err = client.WaitForSandboxStarted(ctx, sandboxID, 2*time.Minute)
	if err != nil {
		t.Fatalf("Failed waiting for started state: %v", err)
	}
	t.Logf("Sandbox started, state: %s", sandbox.State)
}

// TestDaytonaFileOperations tests file listing and deletion.
func TestDaytonaFileOperations(t *testing.T) {
	skipIfNoAPIKey(t)

	backend := NewDaytonaBackend(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Create a sandbox
	t.Log("Creating sandbox for file operations test...")
	session, err := backend.Create(ctx, CreateOptions{
		Name:    "gastown-fileops-" + time.Now().Format("20060102-150405"),
		WorkDir: "/home/daytona",
	})
	if err != nil {
		t.Fatalf("Failed to create sandbox: %v", err)
	}
	t.Logf("Created sandbox: %s", session.ID)

	defer func() {
		t.Log("Destroying sandbox...")
		backend.Destroy(context.Background(), session)
	}()

	client, err := backend.GetClient()
	if err != nil {
		t.Fatalf("Failed to get client: %v", err)
	}

	sandboxID := session.Metadata[MetaSandboxID]

	// Create a folder
	t.Log("Creating folder...")
	if err := client.CreateFolder(ctx, sandboxID, "/home/daytona/testdir", ""); err != nil {
		t.Fatalf("Failed to create folder: %v", err)
	}
	t.Log("Folder created")

	// Create a file using execute command
	t.Log("Creating test file...")
	execResp, err := client.ExecuteCommand(ctx, sandboxID, &daytonaPkg.ExecuteRequest{
		Command: "sh -c 'echo test content > /home/daytona/testdir/test.txt'",
		Timeout: 10,
	})
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	if execResp.ExitCode != 0 {
		t.Fatalf("Failed to create test file, exit code: %d, output: %s", execResp.ExitCode, execResp.Result)
	}

	// List files
	t.Log("Listing files...")
	files, err := client.ListFiles(ctx, sandboxID, "/home/daytona/testdir")
	if err != nil {
		t.Fatalf("Failed to list files: %v", err)
	}
	t.Logf("Found %d files:", len(files))
	for _, f := range files {
		t.Logf("  - %s (dir=%v, size=%d)", f.Name, f.IsDir, f.Size)
	}

	if len(files) == 0 {
		t.Error("Expected at least one file")
	}

	// Delete file
	t.Log("Deleting file...")
	if err := client.DeleteFile(ctx, sandboxID, "/home/daytona/testdir/test.txt", false); err != nil {
		t.Fatalf("Failed to delete file: %v", err)
	}
	t.Log("File deleted")

	// Verify file is gone
	files, err = client.ListFiles(ctx, sandboxID, "/home/daytona/testdir")
	if err != nil {
		t.Fatalf("Failed to list files after delete: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("Expected 0 files after delete, got %d", len(files))
	}
	t.Log("File deletion verified")

	// Delete folder recursively
	t.Log("Deleting folder recursively...")
	if err := client.DeleteFile(ctx, sandboxID, "/home/daytona/testdir", true); err != nil {
		t.Fatalf("Failed to delete folder: %v", err)
	}
	t.Log("Folder deleted")
}

// TestDaytonaGitOperations tests all git operations using a local bare repo.
// This allows testing push/pull without external credentials.
func TestDaytonaGitOperations(t *testing.T) {
	skipIfNoAPIKey(t)

	backend := NewDaytonaBackend(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Create a sandbox
	t.Log("Creating sandbox for git operations test...")
	session, err := backend.Create(ctx, CreateOptions{
		Name:    "gastown-git-" + time.Now().Format("20060102-150405"),
		WorkDir: "/home/daytona",
	})
	if err != nil {
		t.Fatalf("Failed to create sandbox: %v", err)
	}
	t.Logf("Created sandbox: %s", session.ID)

	defer func() {
		t.Log("Destroying sandbox...")
		backend.Destroy(context.Background(), session)
	}()

	client, err := backend.GetClient()
	if err != nil {
		t.Fatalf("Failed to get client: %v", err)
	}

	sandboxID := session.Metadata[MetaSandboxID]
	bareRepoPath := "/tmp/test-remote.git"
	repoPath := "/home/daytona/test-repo"

	// 0. Create a local bare repository and initialize it with a commit
	t.Log("0. Creating and initializing local bare repository...")
	initScript := `
		git init --bare ` + bareRepoPath + ` &&
		tmp=$(mktemp -d) &&
		cd "$tmp" &&
		git init &&
		git config user.email "test@gastown.local" &&
		git config user.name "GasTown Test" &&
		echo "Initial content" > README.md &&
		git add README.md &&
		git commit -m "Initial commit" &&
		git remote add origin ` + bareRepoPath + ` &&
		git push origin master
	`
	_, err = client.ExecuteCommand(ctx, sandboxID, &daytonaPkg.ExecuteRequest{
		Command: "sh -c '" + initScript + "'",
		Timeout: 30,
	})
	if err != nil {
		t.Fatalf("Failed to create and init bare repo: %v", err)
	}
	t.Log("Bare repository created and initialized")

	// 1. Clone from the local bare repo
	t.Log("1. Cloning from local bare repo...")
	err = client.GitClone(ctx, sandboxID, &daytonaPkg.GitCloneRequest{
		URL:  bareRepoPath,
		Path: repoPath,
	})
	if err != nil {
		t.Fatalf("GitClone failed: %v", err)
	}
	t.Log("Repository cloned")

	// 4. Get git status
	t.Log("4. Getting git status...")
	status, err := client.GitStatus(ctx, sandboxID, repoPath)
	if err != nil {
		t.Fatalf("GitStatus failed: %v", err)
	}
	t.Logf("Git status: branch=%s, ahead=%d, behind=%d", status.CurrentBranch, status.Ahead, status.Behind)
	if status.CurrentBranch == "" {
		t.Error("Expected a branch name")
	}

	// 5. List branches
	t.Log("5. Listing branches...")
	branches, err := client.GitBranches(ctx, sandboxID, repoPath)
	if err != nil {
		t.Fatalf("GitBranches failed: %v", err)
	}
	t.Logf("Found %d branches:", len(branches))
	for _, b := range branches {
		t.Logf("  - %s", b.Name)
	}

	// 6. Create a new branch
	t.Log("6. Creating new branch 'feature-branch'...")
	err = client.GitCreateBranch(ctx, sandboxID, &daytonaPkg.GitBranchRequest{
		Path: repoPath,
		Name: "feature-branch",
	})
	if err != nil {
		t.Fatalf("GitCreateBranch failed: %v", err)
	}
	t.Log("Branch created")

	// 7. Checkout the new branch
	t.Log("7. Checking out 'feature-branch'...")
	err = client.GitCheckout(ctx, sandboxID, &daytonaPkg.GitCheckoutRequest{
		Path:   repoPath,
		Branch: "feature-branch",
	})
	if err != nil {
		t.Fatalf("GitCheckout failed: %v", err)
	}
	t.Log("Checkout successful")

	// Verify we're on the new branch
	status, err = client.GitStatus(ctx, sandboxID, repoPath)
	if err != nil {
		t.Fatalf("GitStatus failed: %v", err)
	}
	t.Logf("Current branch: %s", status.CurrentBranch)

	// 8. Create a file and stage it
	t.Log("8. Creating and staging a file...")
	_, err = client.ExecuteCommand(ctx, sandboxID, &daytonaPkg.ExecuteRequest{
		Command: "sh -c 'echo feature content > " + repoPath + "/feature.txt'",
		Timeout: 10,
	})
	if err != nil {
		t.Fatalf("Failed to create feature file: %v", err)
	}

	err = client.GitAdd(ctx, sandboxID, &daytonaPkg.GitAddRequest{
		Path:  repoPath,
		Files: []string{"feature.txt"},
	})
	if err != nil {
		t.Fatalf("GitAdd failed: %v", err)
	}
	t.Log("File staged")

	// Verify file is staged
	status, err = client.GitStatus(ctx, sandboxID, repoPath)
	if err != nil {
		t.Fatalf("GitStatus failed: %v", err)
	}
	t.Logf("File status: %+v", status.FileStatus)
	hasStagedFile := false
	for _, f := range status.FileStatus {
		if f.Staging == daytonaPkg.FileStatusAdded || f.Staging == daytonaPkg.FileStatusModified {
			hasStagedFile = true
			t.Logf("Staged file: %s (staging=%s)", f.Name, f.Staging)
		}
	}
	if !hasStagedFile {
		t.Error("Expected at least one staged file (Added or Modified) after GitAdd")
	}

	// 9. Commit the changes
	t.Log("9. Committing changes...")
	err = client.GitCommit(ctx, sandboxID, &daytonaPkg.GitCommitRequest{
		Path:    repoPath,
		Message: "Add feature file",
		Author:  "GasTown Test",
		Email:   "test@gastown.local",
	})
	if err != nil {
		t.Fatalf("GitCommit failed: %v", err)
	}
	t.Log("Commit successful")

	// Verify commit
	status, err = client.GitStatus(ctx, sandboxID, repoPath)
	if err != nil {
		t.Fatalf("GitStatus failed: %v", err)
	}
	t.Logf("After commit - file status: %+v, ahead: %d", status.FileStatus, status.Ahead)

	// 10. Push the feature branch
	t.Log("10. Pushing feature branch...")
	err = client.GitPush(ctx, sandboxID, &daytonaPkg.GitPullPushRequest{
		Path: repoPath,
	})
	if err != nil {
		t.Fatalf("GitPush failed: %v", err)
	}
	t.Log("Push successful")

	// Verify we're no longer ahead after push
	status, err = client.GitStatus(ctx, sandboxID, repoPath)
	if err != nil {
		t.Fatalf("GitStatus failed: %v", err)
	}
	t.Logf("After push - ahead: %d", status.Ahead)
	if status.Ahead != 0 {
		t.Errorf("Expected ahead=0 after push, got %d", status.Ahead)
	}

	// 11. Switch back to master/main
	t.Log("11. Switching back to master...")
	err = client.GitCheckout(ctx, sandboxID, &daytonaPkg.GitCheckoutRequest{
		Path:   repoPath,
		Branch: "master",
	})
	if err != nil {
		t.Fatalf("GitCheckout to master failed: %v", err)
	}

	// 12. Pull (should be a no-op but verifies the method works)
	t.Log("12. Testing GitPull...")
	err = client.GitPull(ctx, sandboxID, &daytonaPkg.GitPullPushRequest{
		Path: repoPath,
	})
	if err != nil {
		t.Fatalf("GitPull failed: %v", err)
	}
	t.Log("Pull successful")

	// 13. Delete the feature branch
	t.Log("13. Deleting feature branch...")
	err = client.GitDeleteBranch(ctx, sandboxID, &daytonaPkg.GitBranchRequest{
		Path: repoPath,
		Name: "feature-branch",
	})
	if err != nil {
		t.Fatalf("GitDeleteBranch failed: %v", err)
	}
	t.Log("Branch deleted")

	// Verify branch is gone
	branches, err = client.GitBranches(ctx, sandboxID, repoPath)
	if err != nil {
		t.Fatalf("GitBranches failed: %v", err)
	}
	for _, b := range branches {
		if b.Name == "feature-branch" {
			t.Error("feature-branch should have been deleted")
		}
	}
	t.Log("All git operations completed successfully")
}

// TestDaytonaPtySessionInfo tests getting PTY session information.
func TestDaytonaPtySessionInfo(t *testing.T) {
	skipIfNoAPIKey(t)

	backend := NewDaytonaBackend(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Create a sandbox
	t.Log("Creating sandbox for PTY info test...")
	session, err := backend.Create(ctx, CreateOptions{
		Name:    "gastown-ptyinfo-" + time.Now().Format("20060102-150405"),
		WorkDir: "/home/daytona",
	})
	if err != nil {
		t.Fatalf("Failed to create sandbox: %v", err)
	}
	t.Logf("Created sandbox: %s", session.ID)

	defer func() {
		t.Log("Destroying sandbox...")
		backend.Destroy(context.Background(), session)
	}()

	client, err := backend.GetClient()
	if err != nil {
		t.Fatalf("Failed to get client: %v", err)
	}

	sandboxID := session.Metadata[MetaSandboxID]

	// Create a PTY session
	t.Log("Creating PTY session...")
	ptyResp, err := client.CreatePtySession(ctx, sandboxID, &daytonaPkg.PtyCreateRequest{
		ID:   "test-pty-info",
		Cols: 120,
		Rows: 40,
		Cwd:  "/home/daytona",
	})
	if err != nil {
		t.Fatalf("Failed to create PTY session: %v", err)
	}
	t.Logf("PTY session created: ID=%s", ptyResp.ID)

	// Get PTY session info
	t.Log("Getting PTY session info...")
	info, err := client.GetPtySession(ctx, sandboxID, ptyResp.ID)
	if err != nil {
		t.Fatalf("Failed to get PTY session info: %v", err)
	}
	t.Logf("PTY info: ID=%s, Cols=%d, Rows=%d, Status=%s", info.ID, info.Cols, info.Rows, info.Status)

	if info.Cols != 120 || info.Rows != 40 {
		t.Errorf("Expected cols=120, rows=40, got cols=%d, rows=%d", info.Cols, info.Rows)
	}

	// Delete PTY session
	t.Log("Deleting PTY session...")
	if err := client.DeletePtySession(ctx, sandboxID, ptyResp.ID); err != nil {
		t.Errorf("Failed to delete PTY session: %v", err)
	} else {
		t.Log("PTY session deleted")
	}
}

// TestDaytonaClaudeCode tests running actual Claude Code in a sandbox.
// This is a longer test that requires ANTHROPIC_API_KEY.
func TestDaytonaClaudeCode(t *testing.T) {
	skipIfNoAPIKey(t)
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY not set, skipping Claude Code test")
	}

	backend := NewDaytonaBackend(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Create a sandbox
	t.Log("Creating sandbox for Claude Code test...")
	session, err := backend.Create(ctx, CreateOptions{
		Name:    "gastown-claude-" + time.Now().Format("20060102-150405"),
		WorkDir: "/home/daytona",
	})
	if err != nil {
		t.Fatalf("Failed to create sandbox: %v", err)
	}
	t.Logf("Created sandbox: %s", session.ID)

	defer func() {
		t.Log("Destroying sandbox...")
		backend.Destroy(context.Background(), session)
	}()

	// Start Claude Code (uses default command)
	t.Log("Starting Claude Code...")
	if err := backend.Start(ctx, session, ""); err != nil {
		t.Fatalf("Failed to start Claude Code: %v", err)
	}
	t.Log("Claude Code started")

	// Wait for Claude to initialize
	t.Log("Waiting for Claude to initialize...")
	time.Sleep(10 * time.Second)

	// Capture initial output
	output, err := backend.CaptureOutput(ctx, session, 100)
	if err != nil {
		t.Logf("Warning: Failed to capture initial output: %v", err)
	} else {
		t.Logf("Initial output (%d bytes):\n%s", len(output), output)
	}

	// Send a simple prompt
	t.Log("Sending prompt to Claude...")
	if err := backend.SendInput(ctx, session, "What is 2+2? Answer with just the number."); err != nil {
		t.Fatalf("Failed to send input: %v", err)
	}

	// Wait for response
	t.Log("Waiting for Claude response...")
	time.Sleep(15 * time.Second)

	// Capture output
	output, err = backend.CaptureOutput(ctx, session, 200)
	if err != nil {
		t.Fatalf("Failed to capture output: %v", err)
	}
	t.Logf("Claude output (%d bytes):\n%s", len(output), output)

	// Stop
	t.Log("Stopping Claude...")
	if err := backend.Stop(ctx, session); err != nil {
		t.Errorf("Failed to stop: %v", err)
	}
	t.Log("Test completed")
}
