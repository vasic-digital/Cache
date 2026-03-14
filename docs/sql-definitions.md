# SQL Definitions

## Module Overview

This module does not directly use SQL databases for its core functionality.

## Database Usage

### Primary Storage
- **Technology**: Redis and in-memory storage
- **Purpose**: Caching data with TTL policies (fixed, sliding, adaptive)
- **Schema**: No SQL schema is required

### Optional SQL Integration
- Cache warming can be configured to populate from SQL databases
- Distributed cache coordination may use SQL-based locking (via `redsync`)
- Cache statistics can be exported to SQL analytics databases

## Related Modules

For SQL database functionality, see the [Database module](../Database/README.md).

## Migration Notes

If SQL support is added in the future:
1. Create migration scripts in `migrations/` directory
2. Follow versioned migration pattern (`001_initial.sql`, `002_add_feature.sql`)
3. Use the `digital.vasic.database` module for database operations
4. Update this document with schema definitions