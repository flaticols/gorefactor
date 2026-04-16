# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed
- Fix handling of nil values in RPC responses. Vim's NIL sentinel is now properly converted to Lua nil in both error and result messages, preventing unexpected behavior when the plugin receives responses from the refactoring server.
