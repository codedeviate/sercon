# Changelog

## [0.6.0](https://github.com/codedeviate/sercon/compare/v0.5.30...v0.6.0) (2026-05-28)


### Features

* **dts:** declare the Sercon runtime global in emitted .d.ts ([5a017d3](https://github.com/codedeviate/sercon/commit/5a017d350c7e4ad5282e5f6bc50c340f122a6756))
* **scriptengine:** inject Sercon global with per-Run argv ([59a2a63](https://github.com/codedeviate/sercon/commit/59a2a63c276322aebc919a27b35cd7713b1fa1ac))
* **sercon:** pass post-"--" args to scripts as Sercon.argv ([8b9c962](https://github.com/codedeviate/sercon/commit/8b9c9624d7045c7347ed42afec7ba7d5cd935156))


### Bug Fixes

* **sercon:** kill subprocess group on exec.shell timeout ([b293f09](https://github.com/codedeviate/sercon/commit/b293f09a56d939b4aebba73b5a33f67fcb56f807))


### Documentation

* backlog wider database support ([3198c3c](https://github.com/codedeviate/sercon/commit/3198c3c38cbd5253f21faeaa4c6f320f1584fc90))
* **changelog:** finalize 0.6.0 section ([2d43d96](https://github.com/codedeviate/sercon/commit/2d43d968b64dcb92e01194fd9e0dd31119ac7c08))
* document the Sercon runtime global and -- argument passing ([64df2c9](https://github.com/codedeviate/sercon/commit/64df2c9f91f70380856b8536c1444b3e5bec492c))
* **examples:** add argv.ts demonstrating Sercon.argv ([a183b7f](https://github.com/codedeviate/sercon/commit/a183b7f1ff02b58e1411283d9097e5192f6a17f7))
* position sercon as CLI-first, library unsupported ([a1cc2f3](https://github.com/codedeviate/sercon/commit/a1cc2f3cc23ad049e3fa6a3975633abe5fc721ed))
* reframe README around the sercon CLI use case ([3eba50c](https://github.com/codedeviate/sercon/commit/3eba50cdbe59a539ae3b9788ffa0d412996cc03a))
* **sercon:** document -- separator and Sercon.argv in help screens ([ca97603](https://github.com/codedeviate/sercon/commit/ca97603e31e92294a2ab8ff692fb720d9678bee0))


### Build & CI

* allow manual dispatch of the release-please workflow ([65fb76d](https://github.com/codedeviate/sercon/commit/65fb76d3ae37f685abaf4784a9c27eed9fe728f8))
* bump golangci-lint-action v6 -&gt; v7 for golangci-lint v2 ([4378733](https://github.com/codedeviate/sercon/commit/43787330f015721065450edf27915dd67b4c4bd8))
