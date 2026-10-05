import io.kartographer.core.model.ExecutionContext
import io.kartographer.core.model.ExecutionResult
import io.kartographer.core.model.NoInputs
import io.kartographer.core.model.NoOutputs
import io.kartographer.core.model.Task
import io.kartographer.core.model.executeBash
import java.io.File
import java.nio.file.Files
import java.nio.file.StandardCopyOption

/**
 * Kandev's project-local Kartographer plugin.
 *
 * The runtime-bundle and desktop-runtime tasks live here in Kotlin rather than as
 * inline bash in `karto.kts`, so the pipeline file stays a composition of groups.
 *
 * Every binary task is host-only: karto builds and stages just the platform it
 * runs on. Producing a cross-platform bundle (all four remote agentctl helpers)
 * stays a release concern owned by the Make targets.
 */

/** This machine's platform, resolved without shelling out to Go. */
private data class HostPlatform(val goos: String, val goarch: String) {
    /** The remote agentctl helper this host would upload, or null when unsupported. */
    val remoteHelper: String? = when (goos to goarch) {
        "linux" to "amd64" -> "agentctl-linux-amd64"
        "linux" to "arm64" -> "agentctl-linux-arm64"
        "darwin" to "arm64" -> "agentctl-darwin-arm64"
        "darwin" to "amd64" -> "agentctl-darwin-amd64"
        else -> null
    }

    /** The backend Make target that builds [remoteHelper]. */
    val remoteTarget: String? = when (goos to goarch) {
        "linux" to "amd64" -> "build-agentctl-linux"
        "linux" to "arm64" -> "build-agentctl-linux-arm64"
        "darwin" to "arm64" -> "build-agentctl-darwin-arm64"
        "darwin" to "amd64" -> "build-agentctl-darwin-amd64"
        else -> null
    }

    companion object {
        fun detect(): HostPlatform {
            val os = System.getProperty("os.name")?.lowercase() ?: ""
            val arch = System.getProperty("os.arch")?.lowercase() ?: ""
            val goos = when {
                os.contains("linux") -> "linux"
                os.contains("mac") || os.contains("darwin") -> "darwin"
                else -> os.substringBefore(' ')
            }
            val goarch = when (arch) {
                "aarch64", "arm64" -> "arm64"
                "x86_64", "amd64" -> "amd64"
                else -> arch
            }
            return HostPlatform(goos, goarch)
        }
    }
}

/** PATH the Make targets expect: mise/cargo/local shims plus the ambient PATH. */
private fun executionPath(): String {
    val userHome = System.getProperty("user.home") ?: ""
    return listOf(
        "$userHome/.local/share/mise/shims",
        "$userHome/.cargo/bin",
        "$userHome/.local/bin",
        System.getenv("PATH") ?: ""
    ).filter { it.isNotBlank() }.joinToString(":")
}

private fun copyExecutable(source: File, target: File) {
    target.parentFile?.mkdirs()
    Files.copy(source.toPath(), target.toPath(), StandardCopyOption.REPLACE_EXISTING)
    target.setExecutable(true)
}

private suspend fun runMake(context: ExecutionContext, vararg arguments: String): ExecutionResult<NoOutputs> =
    context.executeBash {
        workingDir = context.projectDir.toString()
        environment("PATH", executionPath())
        command("make")
        args(arguments.toList())
    }

/** Builds web + the native launcher/agentctl + the host helper, then assembles dist/kandev/bin. */
private suspend fun buildHostRuntimeBundle(context: ExecutionContext): ExecutionResult<NoOutputs> {
    var result = runMake(context, "-s", "build-web")
    if (result is ExecutionResult.Failure) return result
    result = runMake(context, "-s", "sync-embedded-web")
    if (result is ExecutionResult.Failure) return result
    result = runMake(context, "-C", "apps/backend", "build-kandev", "build-agentctl")
    if (result is ExecutionResult.Failure) return result

    val host = HostPlatform.detect()
    host.remoteTarget?.let { target ->
        val remote = runMake(context, "-C", "apps/backend", target)
        if (remote is ExecutionResult.Failure) return remote
    }

    val projectDir = context.projectDir.toFile()
    val backendBin = File(projectDir, "apps/backend/bin")
    val bundleBin = File(projectDir, "dist/kandev/bin")
    bundleBin.mkdirs()

    // Host-only bundle: drop any remote helper for another platform. The launcher
    // treats remote helpers as optional, so a stale cross-compiled helper must not
    // linger and imply support this build never produced.
    bundleBin.listFiles { file ->
        file.isFile && file.name.startsWith("agentctl-") && file.name != host.remoteHelper
    }?.forEach { it.delete() }

    for (name in listOf("kandev", "agentctl")) {
        copyExecutable(File(backendBin, name), File(bundleBin, name))
    }
    host.remoteHelper?.let { helper ->
        val helperFile = File(backendBin, helper)
        if (helperFile.exists()) copyExecutable(helperFile, File(bundleBin, helper))
    }
    println("Host-only runtime bundle assembled at ${bundleBin.path} (${host.goos}/${host.goarch}).")
    return ExecutionResult.Success(NoOutputs())
}

/** Stages the host-only runtime bundle into the Tauri resource layout. */
private fun stageHostDesktopRuntime(context: ExecutionContext): ExecutionResult<NoOutputs> {
    val projectDir = context.projectDir.toFile()
    val src = File(projectDir, "dist/kandev/bin")
    for (name in listOf("kandev", "agentctl")) {
        val binary = File(src, name)
        if (!binary.exists()) {
            return ExecutionResult.Failure(
                IllegalStateException("Missing ${binary.path}; run 'karto run build/runtime' first."),
                null,
                NoOutputs()
            )
        }
    }

    val out = File(projectDir, "apps/desktop/src-tauri/resources/kandev")
    val outBin = File(out, "bin")
    out.deleteRecursively()
    outBin.mkdirs()
    File(out, ".gitignore").writeText("*\n!.gitignore\n")
    copyExecutable(File(src, "kandev"), File(outBin, "kandev"))
    copyExecutable(File(src, "agentctl"), File(outBin, "agentctl"))
    HostPlatform.detect().remoteHelper?.let { helper ->
        val helperFile = File(src, helper)
        if (helperFile.exists()) copyExecutable(helperFile, File(outBin, helper))
    }
    println("Desktop runtime staged (host-only) at ${out.path}.")
    return ExecutionResult.Success(NoOutputs())
}

/** A human-readable problem for each missing or non-executable host desktop-runtime binary. */
private fun hostDesktopRuntimeProblems(context: ExecutionContext): List<String> {
    val binDir = File(context.projectDir.toFile(), "apps/desktop/src-tauri/resources/kandev/bin")
    val expected = listOf("kandev", "agentctl") + listOfNotNull(HostPlatform.detect().remoteHelper)
    return expected.flatMap { name ->
        val file = File(binDir, name)
        when {
            !file.exists() -> listOf("Missing ${file.path}")
            !file.canExecute() -> listOf("${file.path} is not executable")
            else -> emptyList()
        }
    }
}

/** Builds the host-only runtime bundle consumed by the service and desktop shell. */
class RuntimeBundleTask(name: String) : Task<NoInputs, NoOutputs>(name) {
    val skip = property<Boolean> { set(false) }
        .withCLI("skip", "Skip rebuilding the runtime bundle if dist/kandev/bin/kandev already exists")

    override suspend fun buildInputs(context: ExecutionContext) = NoInputs()
    override fun buildOutputs() = NoOutputs()

    override suspend fun execute(context: ExecutionContext, inputSet: NoInputs): ExecutionResult<NoOutputs> {
        val launcherBin = File(context.projectDir.toFile(), "dist/kandev/bin/kandev")
        if (skip.orNull() == true && launcherBin.exists()) {
            return success()
        }
        return buildHostRuntimeBundle(context)
    }
}

/** Stages dist/kandev into apps/desktop/src-tauri/resources/kandev for the host platform. */
class DesktopStageRuntimeTask(name: String) : Task<NoInputs, NoOutputs>(name) {
    override suspend fun buildInputs(context: ExecutionContext) = NoInputs()
    override fun buildOutputs() = NoOutputs()

    override suspend fun execute(context: ExecutionContext, inputSet: NoInputs): ExecutionResult<NoOutputs> =
        stageHostDesktopRuntime(context)
}

/** Verifies the host-only desktop runtime resources are present and executable. */
class DesktopVerifyTask(name: String) : Task<NoInputs, NoOutputs>(name) {
    override suspend fun buildInputs(context: ExecutionContext) = NoInputs()
    override fun buildOutputs() = NoOutputs()

    override suspend fun execute(context: ExecutionContext, inputSet: NoInputs): ExecutionResult<NoOutputs> {
        val problems = hostDesktopRuntimeProblems(context)
        if (problems.isNotEmpty()) {
            return failure(
                IllegalStateException(
                    problems.joinToString("; ") +
                        ". Run 'karto run desktop/runtime' first to prepare the runtime bundle."
                )
            )
        }
        println("Desktop runtime verified (host-only).")
        return success()
    }
}

/** Builds an optimized profiling build of the desktop app from the host-only runtime. */
class ProfilingBuildTask(name: String) : Task<NoInputs, NoOutputs>(name) {
    val rebuildRuntime = property<Boolean> { set(false) }
        .withCLI("rebuild-runtime", "Force rebuilding the Go runtime bundle before building desktop")

    override suspend fun buildInputs(context: ExecutionContext) = NoInputs()
    override fun buildOutputs() = NoOutputs()

    override suspend fun execute(context: ExecutionContext, inputSet: NoInputs): ExecutionResult<NoOutputs> {
        val launcherBin =
            File(context.projectDir.toFile(), "apps/desktop/src-tauri/resources/kandev/bin/kandev")
        if (rebuildRuntime.orNull() == true || !launcherBin.exists()) {
            val build = buildHostRuntimeBundle(context)
            if (build is ExecutionResult.Failure) return build
            val stage = stageHostDesktopRuntime(context)
            if (stage is ExecutionResult.Failure) return stage
        }

        val problems = hostDesktopRuntimeProblems(context)
        if (problems.isNotEmpty()) {
            return failure(
                IllegalStateException(
                    problems.joinToString("; ") +
                        ". Run 'karto run profiling/build' with --rebuild-runtime."
                )
            )
        }

        return context.executeBash {
            workingDir = context.projectDir.toString()
            environment("PATH", executionPath())
            environment("CARGO_PROFILE_RELEASE_DEBUG", "true")
            command("pnpm")
            args(
                "--filter", "@kandev/desktop", "exec", "tauri", "build",
                "--features", "desktop-runtime,devtools", "--no-bundle"
            )
        }
    }
}
