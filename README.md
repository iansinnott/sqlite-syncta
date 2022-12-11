# Syncta: Sync sqlite databases

**Status:** Unfinished experiment.

## Context

This was an experiment in what would be required to sync to databases
in a semi-general sense. Meaning that you know something about the data
structure or could modify it somewhat in order to achieve the desired sync
results.

However, I had no need for this at the time. It was simply out of curiosity. As
such, it remains unfinished.

Initial thinking:

- For each row, of each table, copy from `src` to `dest`
- In the case of conflict, use a last-write-wins strategy to chose between the
  two rows

Problems:

- How do we know when the last write of a specific row was? Without a cell to
  track an updated timestamp we don't know. Even with a timestamp if it was
  created on different machines (i.e. syncing a database from a remote host) we
  don't have any guarantees the timestamps would match up.
  - one could add `updated_at` fields to everything. indeed this was my initial
    thought, but it's so inelegant. If you have a many-to-many join table it
    feels awkward to add an `updated_at` field to each edge that would otherwise
    just be two foreign key fields.
- What if migrations are not up to date on both databases? Just assume they are.
  This is not a migration tool.

Learnings:

- Hadn't played around with Sqlite introspection prior to this
  - Most notably `PRAGMA table_info(table_name)`
  - Check out the `TableInfo` struct for what is extracted
