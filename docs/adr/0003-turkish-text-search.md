# ADR-0003: unicode61 tokenising, and what it does not fold

**Status:** accepted · **Date:** 2026-10-06

## Context

Notes are written in Turkish and English, often mixed, and often from a
keyboard that is not set up for Turkish. FTS5 ships `unicode61`, `ascii` and
`porter`; none is Turkish-aware, and a custom tokeniser is not reachable from
a pure-Go driver without writing one.

## Decision

Tokenise with `unicode61 remove_diacritics 2`.

## Consequences

- A diacritic folds onto its base letter in both the index and the query, so
  `dugum` finds `düğüm` and `cozulemedi` finds `çözülemedi`. Case folds too.
- **`ı` does not fold to `i`.** U+0131 is its own letter in Unicode, not an `i`
  carrying a mark, so nothing decomposes: `veritabani` does not find
  `veritabanı`. `TestSearchDoesNotFoldDotlessI` pins this so it is a known
  limit rather than a bug report.
- There is no stemming, so `kilitlendi` does not find `kilitlenmek`. Turkish is
  agglutinative and this will be felt. Prefix search (`SearchQuery.Prefix`)
  covers the common case of a word the user has not finished typing.
- Fixing either properly means a Turkish-aware tokeniser or a stemmer, which
  would be a new dependency and so needs its own decision.
