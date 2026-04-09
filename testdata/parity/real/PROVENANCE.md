# Real-world parity fixtures

Each subdirectory contains a pinned snapshot of a small, permissively
licensed real-world project, used by the parity harness to catch
real-world parser bugs that synthetic fixtures would miss.

| Language   | Project       | License | Pin      | Source URL |
|------------|---------------|---------|----------|------------|
| Go         | cvalent       | Apache  | (self)   | this repo  |
| Java       | (pending)     | Apache  | TBD      | TBD        |
| Python     | python-dotenv | BSD     | TBD      | TBD        |
| TypeScript | tiny-glob     | MIT     | TBD      | TBD        |

Fixtures are vendored with `vendor-parity-fixtures.sh` (not yet
written). They must be re-vendored when the pin revision changes.
