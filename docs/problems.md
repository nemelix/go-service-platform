**English** | [Русский](problems.ru.md)

# Problems I ran into

Problems I hit while building this project: what I saw, why it happened, what I did and what I took from it. The most interesting ones are also in the [README](../README.md).

## Deploy and Docker

### The server was running an old version even though the deploy went fine

**Symptom:** CI was green, the deploy finished without errors, but the server answered with the previous version.

**Cause:** images were tagged only as `latest`. The server and the registry had different images under the same `latest`, and the tag couldn't tell which one was running.

**Fix:** every image gets a tag with the commit sha, and the deploy script accepts only a sha. Now the tag tells exactly what code is running, and a rollback is just a deploy of an older sha.

**Lesson:** mutable tags like `latest` don't belong in what gets deployed to a server.

### The container crashed with `exec /server: no such file or directory`, while the file was there

**Symptom:** the image built fine, but the container exited right away with this error. `/server` was in the image.

**Cause:** without `CGO_ENABLED=0` Go built a dynamic binary that needs libc and the dynamic loader. distroless/static has neither, so the kernel can't start the file. "No such file" is about the loader, not the binary.

**How I found it:** `ldd server` on the build stage showed libc dependencies.

**Fix:** `CGO_ENABLED=0` at build time. The result is a static binary that needs nothing.

**Lesson:** "no such file" for a file that is clearly there almost always means the loader, the architecture or CRLF in a shebang.

### Old images were not removed after deploys

**Symptom:** after a few deploys app images piled up on the server and took disk space.

**Cause:** `docker image prune` only removes untagged (dangling) images, and every one of mine has a sha tag.

**Fix:** `docker image prune -af` filtered by the `org.opencontainers.image.source` label set in the Dockerfile. It removes all unused images of this project and leaves everything else alone.

**Lesson:** the Dockerfile label isn't just for GitHub, it's a handy way to select my own images.

## Network and SSH

### A container port was reachable from the internet although ufw blocked it

**Symptom:** a port published with `-p` was open from outside, although ufw didn't allow it.

**Cause:** Docker publishes ports with its own iptables rules (DNAT). That traffic goes through the FORWARD chain, while ufw rules sit in INPUT, so the packets never reach them.

**Fix:** only Nginx is published (80/443). The app is reachable only inside the Docker network, Nginx talks to it by service name.

**Lesson:** with Docker, the host firewall doesn't protect published ports. The protection is publishing nothing extra, binding to `127.0.0.1`, or rules in the `DOCKER-USER` chain.

### SSH password login was on, although `sshd_config` said `no`

**Symptom:** `/etc/ssh/sshd_config` had `PasswordAuthentication no`, but the server accepted a password.

**Cause:** `sshd_config` includes the files from `sshd_config.d/` at the very top. `50-cloud-init.conf` had `PasswordAuthentication yes`, and sshd uses the **first** value it finds, not the last.

**How I found it:** `sshd -T` prints the effective settings sshd actually uses.

**Fix:** fixed the value in `50-cloud-init.conf` and checked it with `sshd -T`.

**Lesson:** I check the effective config with a command, not by reading one file.

### I didn't want to give GitHub full access to my server

**Task:** CI has to deploy to the server, but a key stored in GitHub is a key to the server for anyone who gets it.

**Solution:**
- a separate `deploy` user with no password and no sudo;
- its key in `authorized_keys` is limited with `restrict` and a forced command: any connection runs only the deploy script;
- the script is owned by root and accepts only a 40-character commit sha.

**Lesson:** even if the key leaks, all it can do is deploy one of the images that were already built.

## CI

### The linter found a real bug

**Symptom:** golangci-lint (errcheck) flagged the unchecked error from `http.ListenAndServe` in `main.go`.

**How I checked:** I ran two containers in one network namespace (`--network container:`), so the second one couldn't take port 8080. The second container ended up `Exited (0)` with an empty log, as if it had finished normally.

**Cause:** `ListenAndServe` returned "address already in use", nobody checked it, `main` returned and the process exited with code 0.

**Fix:** `log.Fatalln("error:", http.ListenAndServe(...))`: the error is logged and the exit code is 1.

**Lesson:** errcheck isn't nitpicking. A swallowed error turns a crash into a "successful exit".

### golangci-lint passed locally but failed in CI

**Symptom:** zero issues locally, but in CI:
```
could not import internal/goarch (... export data version 5 is greater than maximum supported version 4) (typecheck)
```

**Cause:** locally I ran `latest`, while CI had v2.13 pinned, built before Go 1.27 came out. It couldn't read the standard library of the newer Go.

**Fix:** pinned v2.15, the version `latest` pointed to.

**Lesson:** I update the linter together with Go. Dependabot won't do it: in the workflow it's just a string under `with:`.

### A PR could be merged although the linter failed

**Symptom:** the `lint` job was red, but the Merge button was green.

**Cause:** only `build` was a required check in the ruleset. When `lint` failed, `build` (which depends on `lint` through `needs`) got the status skipped, and GitHub treats a skipped required check as passed.

**Fix:** made every job a required check: test, golangci-lint, hadolint, shellcheck, gitleaks, build.

**Lesson:** when a job is added or renamed, the list of required checks has to be updated too.

### Trivy stopped the image from being published

**Symptom:** the `build` job failed at the Trivy step, so the push to ghcr and the deploy didn't run.

**Cause:** a HIGH vulnerability in `crypto/x509` from the Go 1.27.2 standard library. I don't call x509 myself (Nginx terminates TLS), but the package is compiled in through `net/http`, and the scanner can't tell which code is actually called.

**Fix:** the fix was in a patch release. Bumped Go to 1.27.3 in the Dockerfile and `go.mod`.

**Lesson:** vulnerabilities in a Go app are most often not in my code but in the standard library, and a Go update fixes them. Unfixed ones (`ignore-unfixed`) don't block the build, otherwise CI would be red all the time.

### GitHub Actions tags turned out to be unreliable

**What happened:** in March 2026 almost all tags of `aquasecurity/trivy-action` were overwritten with malicious code that stole CI secrets. Everyone using `uses: aquasecurity/trivy-action@v0.x` got someone else's code without changing a line on their side.

**Fix:** third-party actions are pinned to the full commit sha with the version in a comment: `uses: owner/repo@<sha> # v1.2.3`. A tag can be moved to another commit, a sha can't. I look the sha up with `git ls-remote`: for an annotated tag it's the line with `^{}`, which is the commit, not the tag object. Dependabot sends updates.

**Lesson:** `uses:` runs someone else's code with access to secrets, and it deserves the same care as code dependencies.
