# CLAUDE.md Template for CodeValent

Copy the section below into your project's CLAUDE.md file.

---

## CodeValent Code Graph

This project has a CodeValent code graph (`.cvalent/` directory). The `cvalent` MCP server provides structural analysis tools.

### Session start
- Call `graph_summary` to understand codebase structure: modules, function counts, edge density, untested functions.

### After context compaction
- Call `graph_summary` again to restore orientation -- you've lost prior context about the codebase structure.

### Before architecture questions
- Call `domains` to see module groupings.
- Call `coupling` to see cross-module dependencies.
- Call `exports <module>` for a module's public API.

### Before changing a function
- Call `impact <function>` to see blast radius before editing.
- Call `callers <function>` to find all call sites.
- Call `contract <function>` to understand expected inputs/outputs.

### When investigating bugs
- Call `callers <function>` to trace who calls the buggy code.
- Call `breaks <function>` to find callers with mismatched data shapes.

### Finding test gaps
- Call `untested` to find functions without test coverage.
- Call `test_coverage <function>` to see which tests exercise a function.
