# Bundled release fingerprints

Each manifest records the upstream release version, the installed path of each regular file in its ZIP, and the SHA-256 digest of that file. WeakAuras ships five addon folders in each of these release assets.

| Upstream ZIP | SHA-256 of the complete ZIP | Files |
| --- | --- | ---: |
| [WeakAuras 5.21.1](https://github.com/WeakAuras/WeakAuras2/releases/download/5.21.1/WeakAuras-5.21.1.zip) | `02266d4e648f999d41c431b5a951b67645b983bb7068e40437f9dbf99c8ad115` | 742 |
| [WeakAuras 5.22.1](https://github.com/WeakAuras/WeakAuras2/releases/download/5.22.1/WeakAuras-5.22.1.zip) | `d0d4406b62e84d5e023f2af95e93d229907a41074d2749dd46643c8843536c33` | 741 |

The ZIP checksums matched the digests GitHub published in the release asset metadata. [5.21.1 release metadata](https://api.github.com/repos/WeakAuras/WeakAuras2/releases/tags/5.21.1) · [5.22.1 release metadata](https://api.github.com/repos/WeakAuras/WeakAuras2/releases/tags/5.22.1)

To reproduce a manifest, download the corresponding ZIP, verify its checksum, and run the maintainer tool from the repository root.

```sh
version=5.21.1
go run ./tools/indexbuild fingerprint --zip "WeakAuras-$version.zip" \
  --version "$version" --out "internal/fingerprint/data/weakauras-$version.json"
```

For a signed catalog, add the generated manifest object to the source package’s optional `fingerprints` array in `candidates.json`. The `indexbuild build` command writes format 2 when an included package carries a manifest. Each release’s top-level folders must be listed in the package’s `folders` field.
