the most useful pattern I've found in two weeks of playing with beads is to say "do this list of tasks, and file bugs against it when you encounter failures." this works real good when you're working on gastown itself, because you encounter a lot of failures, and you want to fix or explain them all.. so that's what ive been using it for; but maybe you could have multiple epics at once, and work on more than just gastown..?  issues would land in wrong epics, you'd need to tease that out.  I partially implemented an idea where each crewmember own a formula, but maybe each crew should actually own an epic, and primarily be responsible for maintaining its contents and completing work via polecats.

I like that idea, how would you effectuate it?   You'd modify the crew's agent bead to have something like this:

```
Your primary repsonsibility is the epic hooked to you: to complete your task, the epic must be researched, designed, implemented, tested, and integrated into our gastown. 
The way you will work through this epic, your primum mobile, is the principle of "If you Fail, then you File".  How does this work?
1. <FAIL> when you or your polecats encounter an issue, error, bug, hidrance, failure, or mistake
2. <FILE> you must immediately create a tracking bug and assign it to an epic

Preferentially assign bugs to
1. your epic
2. another existing epic
3. the "Untracked Work" epic (create if needed)

But DO NOT
1. create a new epic

As you work on your epic you will add many tasks to it; the best way to complete these tasks is by creating polecats.  You should peek at your polecats while they are running.  many valuable <FAIL>s can be <FILE>d using this information.
```

This 
And the way we are going to bootstrap this process, is to create a crew responsible for the "file after fail" epic (maybe we can come up with a better name).  and what that crew will do, is, they will figure out how we're going to effectuate my idea.   So here's what you'll do:1. create a "File After Fail" epic; this epic always should start with the prompt above, which is the initial version of the "File After Fail" principle, and we should record that fact in the epic above.  
2. The file_after_fail crew must always strive to be a good file after failer itself.



## bd-3q6.6-1: Foreign key violation when slinging cross-rig beads

**Fixed in beads repo commit:** dd21ae43

**Problem:** The Dolt schema had a foreign key constraint on `dependencies.depends_on_id`
that referenced `issues.id`. This prevented storing external references
(`external:<project>:<capability>`) which by design don't exist in the local issues table.

**Solution:**
1. Removed the FK constraint from the Dolt schema (new databases)
2. Added an idempotent migration to drop the FK from existing databases
3. Matches the SQLite migration `025_remove_depends_on_fk.go`

**Files changed (in beads repo):**
- `internal/storage/dolt/schema.go` - Removed FK constraint
- `internal/storage/dolt/store.go` - Added migration to drop existing FK
