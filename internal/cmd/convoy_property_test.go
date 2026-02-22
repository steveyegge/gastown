package cmd

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"
)

// randomAcyclicDAG generates a random DAG guaranteed to be acyclic.
// Nodes are numbered 0..n-1. Edges only go from lower to higher numbered nodes,
// ensuring acyclicity by construction. Then edges are randomly selected.
// All nodes are type "task" with rig "gastown".
func randomAcyclicDAG(seed int64, nodes, maxEdges int) *ConvoyDAG {
	rng := rand.New(rand.NewSource(seed))
	dag := &ConvoyDAG{Nodes: make(map[string]*ConvoyDAGNode)}

	// Create nodes
	for i := 0; i < nodes; i++ {
		id := fmt.Sprintf("t-%d", i)
		dag.Nodes[id] = &ConvoyDAGNode{
			ID:    id,
			Title: fmt.Sprintf("Task %d", i),
			Type:  "task",
			Rig:   "gastown",
		}
	}

	// Add random edges (only forward: from lower to higher index)
	edgesAdded := 0
	for edgesAdded < maxEdges {
		from := rng.Intn(nodes)
		to := rng.Intn(nodes)
		if from >= to {
			continue // only forward edges
		}

		fromID := fmt.Sprintf("t-%d", from)
		toID := fmt.Sprintf("t-%d", to)

		// Avoid duplicate edges
		fromNode := dag.Nodes[fromID]
		alreadyExists := false
		for _, b := range fromNode.Blocks {
			if b == toID {
				alreadyExists = true
				break
			}
		}
		if alreadyExists {
			continue
		}

		fromNode.Blocks = append(fromNode.Blocks, toID)
		dag.Nodes[toID].BlockedBy = append(dag.Nodes[toID].BlockedBy, fromID)
		edgesAdded++
	}

	return dag
}

// randomDAGWithCycle generates a DAG then injects a back-edge to create a cycle.
// First ensures a forward path 0→1→...→(n-1) exists, then adds (n-1)→0 back-edge.
func randomDAGWithCycle(seed int64, nodes int) *ConvoyDAG {
	// Start with acyclic DAG
	dag := randomAcyclicDAG(seed, nodes, nodes/2)

	// Ensure a forward path 0→1→2→...→(n-1) so the back-edge creates a guaranteed cycle.
	for i := 0; i < nodes-1; i++ {
		fromID := fmt.Sprintf("t-%d", i)
		toID := fmt.Sprintf("t-%d", i+1)
		fromNode := dag.Nodes[fromID]

		// Skip if edge already exists.
		exists := false
		for _, b := range fromNode.Blocks {
			if b == toID {
				exists = true
				break
			}
		}
		if exists {
			continue
		}
		fromNode.Blocks = append(fromNode.Blocks, toID)
		dag.Nodes[toID].BlockedBy = append(dag.Nodes[toID].BlockedBy, fromID)
	}

	// Inject a back-edge from the last node to the first to close the cycle.
	lastID := fmt.Sprintf("t-%d", nodes-1)
	firstID := "t-0"

	dag.Nodes[lastID].Blocks = append(dag.Nodes[lastID].Blocks, firstID)
	dag.Nodes[firstID].BlockedBy = append(dag.Nodes[firstID].BlockedBy, lastID)

	return dag
}

// TestRandomDAG_AcyclicIsAcyclic verifies generated acyclic DAGs have no cycles
func TestRandomDAG_AcyclicIsAcyclic(t *testing.T) {
	for seed := int64(0); seed < 50; seed++ {
		dag := randomAcyclicDAG(seed, 20, 30)
		cycle := detectCycles(dag)
		if cycle != nil {
			t.Fatalf("seed %d: acyclic DAG has cycle: %v", seed, cycle)
		}
	}
}

// TestRandomDAG_CyclicHasCycle verifies generated cyclic DAGs have cycles
func TestRandomDAG_CyclicHasCycle(t *testing.T) {
	for seed := int64(0); seed < 50; seed++ {
		dag := randomDAGWithCycle(seed, 10)
		cycle := detectCycles(dag)
		if cycle == nil {
			t.Fatalf("seed %d: cyclic DAG has no cycle detected", seed)
		}
	}
}

// TestRandomDAG_WavesTerminate verifies wave computation always terminates on acyclic DAGs
func TestRandomDAG_WavesTerminate(t *testing.T) {
	for seed := int64(0); seed < 50; seed++ {
		dag := randomAcyclicDAG(seed, 15, 20)
		waves, err := computeWaves(dag)
		if err != nil {
			t.Fatalf("seed %d: computeWaves error: %v", seed, err)
		}
		if len(waves) == 0 {
			t.Fatalf("seed %d: expected at least 1 wave", seed)
		}
		if len(waves[0].Tasks) == 0 {
			t.Fatalf("seed %d: Wave 1 is empty", seed)
		}
	}
}

// TestRandomDAG_Deterministic verifies same seed produces same waves
func TestRandomDAG_Deterministic(t *testing.T) {
	for seed := int64(0); seed < 20; seed++ {
		dag1 := randomAcyclicDAG(seed, 10, 15)
		dag2 := randomAcyclicDAG(seed, 10, 15)
		waves1, _ := computeWaves(dag1)
		waves2, _ := computeWaves(dag2)

		if len(waves1) != len(waves2) {
			t.Fatalf("seed %d: different wave counts: %d vs %d", seed, len(waves1), len(waves2))
		}
		for i := range waves1 {
			if len(waves1[i].Tasks) != len(waves2[i].Tasks) {
				t.Fatalf("seed %d wave %d: different task counts", seed, i)
			}
			for j := range waves1[i].Tasks {
				if waves1[i].Tasks[j] != waves2[i].Tasks[j] {
					t.Fatalf("seed %d wave %d task %d: %s vs %s", seed, i, j, waves1[i].Tasks[j], waves2[i].Tasks[j])
				}
			}
		}
	}
}

// TestRandomDAG_SeedLoggedOnFailure demonstrates seed logging pattern
func TestRandomDAG_SeedLoggedOnFailure(t *testing.T) {
	// This test should always pass — it just verifies the pattern works
	seed := int64(42)
	dag := randomAcyclicDAG(seed, 5, 5)
	cycle := detectCycles(dag)
	if cycle != nil {
		t.Fatalf("REPRODUCE: seed=%d nodes=5 edges=5 — cycle: %v", seed, cycle)
	}
}

// ---------------------------------------------------------------------------
// PT-01 through PT-06: Property-based tests over randomized DAGs
// ---------------------------------------------------------------------------

// TestProperty_WaveComputationTerminates (PT-01) verifies computeWaves always
// terminates, returns at least 1 wave, and Wave 1 is non-empty for acyclic DAGs
// of varying sizes.
func TestProperty_WaveComputationTerminates(t *testing.T) {
	for seed := int64(100); seed < 200; seed++ {
		// Vary node count between 5 and 50, edges = nodes*2
		nodes := 5 + int(seed-100)%46
		edges := nodes * 2
		dag := randomAcyclicDAG(seed, nodes, edges)

		waves, err := computeWaves(dag)
		if err != nil {
			t.Fatalf("REPRODUCE: seed=%d nodes=%d edges=%d — computeWaves error: %v", seed, nodes, edges, err)
		}
		if len(waves) == 0 {
			t.Fatalf("REPRODUCE: seed=%d nodes=%d edges=%d — expected at least 1 wave, got 0", seed, nodes, edges)
		}
		if len(waves[0].Tasks) == 0 {
			t.Fatalf("REPRODUCE: seed=%d nodes=%d edges=%d — Wave 1 is empty", seed, nodes, edges)
		}
	}
}

// TestProperty_EveryTaskInExactlyOneWave (PT-02) verifies every slingable task
// appears in exactly one wave — no duplicates, no omissions.
func TestProperty_EveryTaskInExactlyOneWave(t *testing.T) {
	for seed := int64(200); seed < 300; seed++ {
		nodes := 5 + int(seed-200)%46
		edges := nodes * 2
		dag := randomAcyclicDAG(seed, nodes, edges)

		waves, err := computeWaves(dag)
		if err != nil {
			t.Fatalf("REPRODUCE: seed=%d — computeWaves error: %v", seed, err)
		}

		// Collect all task IDs from waves (including potential duplicates).
		var allWaveTaskIDs []string
		uniqueWaveTasks := make(map[string]bool)
		for _, wave := range waves {
			for _, id := range wave.Tasks {
				allWaveTaskIDs = append(allWaveTaskIDs, id)
				if uniqueWaveTasks[id] {
					t.Fatalf("REPRODUCE: seed=%d — duplicate task %q in waves", seed, id)
				}
				uniqueWaveTasks[id] = true
			}
		}

		// Collect all slingable node IDs from the DAG.
		slingableIDs := make(map[string]bool)
		for id, node := range dag.Nodes {
			if isSlingableType(node.Type) {
				slingableIDs[id] = true
			}
		}

		// Every slingable node must appear in waves.
		for id := range slingableIDs {
			if !uniqueWaveTasks[id] {
				t.Fatalf("REPRODUCE: seed=%d — slingable task %q missing from waves", seed, id)
			}
		}
		// Every wave task must be a slingable node.
		for id := range uniqueWaveTasks {
			if !slingableIDs[id] {
				t.Fatalf("REPRODUCE: seed=%d — wave task %q is not slingable in DAG", seed, id)
			}
		}
		// Counts must match (belt + suspenders with the above checks).
		if len(allWaveTaskIDs) != len(slingableIDs) {
			t.Fatalf("REPRODUCE: seed=%d — wave task count %d != slingable count %d",
				seed, len(allWaveTaskIDs), len(slingableIDs))
		}
	}
}

// TestProperty_NoTaskBeforeBlocker (PT-03) verifies no task is scheduled in a
// wave before all of its blockers. For every slingable node, each slingable
// blocker must have a strictly earlier wave number.
func TestProperty_NoTaskBeforeBlocker(t *testing.T) {
	for seed := int64(300); seed < 400; seed++ {
		nodes := 10 + int(seed-300)%41
		edges := nodes * 3 // higher edge count for denser graphs
		dag := randomAcyclicDAG(seed, nodes, edges)

		waves, err := computeWaves(dag)
		if err != nil {
			t.Fatalf("REPRODUCE: seed=%d — computeWaves error: %v", seed, err)
		}

		// Build taskID → waveNumber map.
		taskWave := make(map[string]int)
		for _, wave := range waves {
			for _, id := range wave.Tasks {
				taskWave[id] = wave.Number
			}
		}

		// Check every slingable node's blockers.
		for id, node := range dag.Nodes {
			if !isSlingableType(node.Type) {
				continue
			}
			myWave, ok := taskWave[id]
			if !ok {
				continue // should be covered by PT-02
			}
			for _, blockerID := range node.BlockedBy {
				blockerNode := dag.Nodes[blockerID]
				if blockerNode == nil || !isSlingableType(blockerNode.Type) {
					continue
				}
				blockerWave, ok := taskWave[blockerID]
				if !ok {
					continue
				}
				if blockerWave >= myWave {
					t.Fatalf("REPRODUCE: seed=%d — task %q (wave %d) has blocker %q in wave %d (must be strictly earlier)",
						seed, id, myWave, blockerID, blockerWave)
				}
			}
		}
	}
}

// TestProperty_CycleAlwaysDetectedInCyclicDAGs (PT-04) verifies detectCycles
// always finds a cycle in cyclic DAGs, the cycle has >= 2 nodes, and each
// consecutive pair has a Blocks edge.
func TestProperty_CycleAlwaysDetectedInCyclicDAGs(t *testing.T) {
	for seed := int64(400); seed < 500; seed++ {
		nodes := 5 + int(seed-400)%26 // 5-30 nodes
		dag := randomDAGWithCycle(seed, nodes)

		cycle := detectCycles(dag)
		if cycle == nil {
			t.Fatalf("REPRODUCE: seed=%d nodes=%d — no cycle detected in cyclic DAG", seed, nodes)
		}
		if len(cycle) < 2 {
			t.Fatalf("REPRODUCE: seed=%d nodes=%d — cycle path too short: %v", seed, nodes, cycle)
		}

		// Verify each consecutive pair in the cycle has a Blocks edge.
		for i := 0; i < len(cycle); i++ {
			from := cycle[i]
			to := cycle[(i+1)%len(cycle)]
			fromNode := dag.Nodes[from]
			if fromNode == nil {
				t.Fatalf("REPRODUCE: seed=%d — cycle node %q not in DAG", seed, from)
			}
			hasEdge := false
			for _, b := range fromNode.Blocks {
				if b == to {
					hasEdge = true
					break
				}
			}
			if !hasEdge {
				t.Fatalf("REPRODUCE: seed=%d — cycle edge %q→%q has no Blocks edge in DAG (cycle: %v)",
					seed, from, to, cycle)
			}
		}
	}
}

// TestProperty_DeterministicWaveAssignment (PT-05) verifies same seed always
// produces identical wave assignments: same wave count, same tasks, same order.
func TestProperty_DeterministicWaveAssignment(t *testing.T) {
	for seed := int64(500); seed < 600; seed++ {
		nodes := 10 + int(seed-500)%41
		edges := nodes * 2

		dag1 := randomAcyclicDAG(seed, nodes, edges)
		dag2 := randomAcyclicDAG(seed, nodes, edges)

		waves1, err1 := computeWaves(dag1)
		waves2, err2 := computeWaves(dag2)

		if (err1 == nil) != (err2 == nil) {
			t.Fatalf("REPRODUCE: seed=%d — error mismatch: %v vs %v", seed, err1, err2)
		}
		if err1 != nil {
			continue
		}

		if len(waves1) != len(waves2) {
			t.Fatalf("REPRODUCE: seed=%d — wave count mismatch: %d vs %d", seed, len(waves1), len(waves2))
		}

		for i := range waves1 {
			if waves1[i].Number != waves2[i].Number {
				t.Fatalf("REPRODUCE: seed=%d wave %d — number mismatch: %d vs %d",
					seed, i, waves1[i].Number, waves2[i].Number)
			}
			if len(waves1[i].Tasks) != len(waves2[i].Tasks) {
				t.Fatalf("REPRODUCE: seed=%d wave %d — task count mismatch: %d vs %d",
					seed, i, len(waves1[i].Tasks), len(waves2[i].Tasks))
			}
			for j := range waves1[i].Tasks {
				if waves1[i].Tasks[j] != waves2[i].Tasks[j] {
					t.Fatalf("REPRODUCE: seed=%d wave %d task %d — %q vs %q",
						seed, i, j, waves1[i].Tasks[j], waves2[i].Tasks[j])
				}
			}
		}
	}
}

// TestProperty_ParentChildDoesNotAffectWaves (PT-06) verifies that adding
// parent-child hierarchy edges (epics as parents) does not change wave
// assignments for slingable tasks.
func TestProperty_ParentChildDoesNotAffectWaves(t *testing.T) {
	for seed := int64(600); seed < 650; seed++ {
		nodes := 10 + int(seed-600)%41
		edges := nodes * 2
		rng := rand.New(rand.NewSource(seed))

		// Generate a base DAG (slingable tasks only, no parent-child).
		dag1 := randomAcyclicDAG(seed, nodes, edges)
		waves1, err := computeWaves(dag1)
		if err != nil {
			t.Fatalf("REPRODUCE: seed=%d — computeWaves (no parents) error: %v", seed, err)
		}

		// Generate the same DAG, then add epic parents.
		dag2 := randomAcyclicDAG(seed, nodes, edges)

		// Add some epic nodes and assign them as parents of random tasks.
		numEpics := 1 + rng.Intn(3) // 1-3 epics
		epicIDs := make([]string, numEpics)
		for i := 0; i < numEpics; i++ {
			epicID := fmt.Sprintf("epic-%d", i)
			epicIDs[i] = epicID
			dag2.Nodes[epicID] = &ConvoyDAGNode{
				ID:    epicID,
				Title: fmt.Sprintf("Epic %d", i),
				Type:  "epic",
				Rig:   "gastown",
			}
		}

		// Assign random tasks to random epic parents.
		taskIDs := make([]string, 0, nodes)
		for id := range dag2.Nodes {
			if isSlingableType(dag2.Nodes[id].Type) {
				taskIDs = append(taskIDs, id)
			}
		}
		sort.Strings(taskIDs) // deterministic iteration
		for _, taskID := range taskIDs {
			if rng.Float64() < 0.6 { // 60% chance of having a parent
				epicID := epicIDs[rng.Intn(len(epicIDs))]
				dag2.Nodes[taskID].Parent = epicID
				dag2.Nodes[epicID].Children = append(dag2.Nodes[epicID].Children, taskID)
			}
		}

		waves2, err := computeWaves(dag2)
		if err != nil {
			t.Fatalf("REPRODUCE: seed=%d — computeWaves (with parents) error: %v", seed, err)
		}

		// Wave assignments for tasks must be identical.
		if len(waves1) != len(waves2) {
			t.Fatalf("REPRODUCE: seed=%d — wave count changed: %d vs %d after adding parent-child edges",
				seed, len(waves1), len(waves2))
		}
		for i := range waves1 {
			if waves1[i].Number != waves2[i].Number {
				t.Fatalf("REPRODUCE: seed=%d wave %d — number changed: %d vs %d",
					seed, i, waves1[i].Number, waves2[i].Number)
			}
			if len(waves1[i].Tasks) != len(waves2[i].Tasks) {
				t.Fatalf("REPRODUCE: seed=%d wave %d — task count changed: %d vs %d",
					seed, i, len(waves1[i].Tasks), len(waves2[i].Tasks))
			}
			for j := range waves1[i].Tasks {
				if waves1[i].Tasks[j] != waves2[i].Tasks[j] {
					t.Fatalf("REPRODUCE: seed=%d wave %d task %d — %q vs %q after adding parent-child edges",
						seed, i, j, waves1[i].Tasks[j], waves2[i].Tasks[j])
				}
			}
		}

		// Verify epics do NOT appear in any wave.
		for _, wave := range waves2 {
			for _, id := range wave.Tasks {
				if dag2.Nodes[id] != nil && dag2.Nodes[id].Type == "epic" {
					t.Fatalf("REPRODUCE: seed=%d — epic %q appeared in wave %d", seed, id, wave.Number)
				}
			}
		}
	}
}
