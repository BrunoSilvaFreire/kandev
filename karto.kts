import io.kartographer.core.model.*
import io.kartographer.dsl.*
import java.io.File
import java.nio.file.Files
import java.time.LocalDateTime
import java.time.format.DateTimeFormatter

project {
    rootDirectory = "."
}

/**
 * Builds the web frontend application (@kandev/web).
 */
open class WebBuildTask(name: String) : Task<NoInputs, NoOutputs>(name) {
    private fun executionPath(): String {
        val userHome = System.getProperty("user.home") ?: ""
        return listOf(
            "$userHome/.local/share/mise/shims",
            "$userHome/.cargo/bin",
            "$userHome/.local/bin",
            System.getenv("PATH") ?: ""
        ).filter { it.isNotBlank() }.joinToString(":")
    }

    override suspend fun buildInputs(context: ExecutionContext) = NoInputs()
    override fun buildOutputs() = NoOutputs()

    override suspend fun execute(context: ExecutionContext, inputSet: NoInputs): ExecutionResult<NoOutputs> {
        return context.executeBash {
            workingDir = context.projectDir.toString()
            environment("PATH", executionPath())
            command("bash")
            args("-c", "make -s build-web")
        }
    }
}

/**
 * Builds the Go backend binary (apps/backend).
 */
open class BackendBuildTask(name: String) : Task<NoInputs, NoOutputs>(name) {
    private fun executionPath(): String {
        val userHome = System.getProperty("user.home") ?: ""
        return listOf(
            "$userHome/.local/share/mise/shims",
            "$userHome/.cargo/bin",
            "$userHome/.local/bin",
            System.getenv("PATH") ?: ""
        ).filter { it.isNotBlank() }.joinToString(":")
    }

    override suspend fun buildInputs(context: ExecutionContext) = NoInputs()
    override fun buildOutputs() = NoOutputs()

    override suspend fun execute(context: ExecutionContext, inputSet: NoInputs): ExecutionResult<NoOutputs> {
        return context.executeBash {
            workingDir = context.projectDir.toString()
            environment("PATH", executionPath())
            command("bash")
            args("-c", "make -C apps/backend build")
        }
    }
}

/**
 * Builds the Tauri desktop bundle.
 * Defaults to '--bundles deb' for fast iteration, avoiding slow RPM compression.
 */
open class DesktopBuildTask(name: String) : Task<NoInputs, NoOutputs>(name) {
    val bundles = property<String> {
        set("deb")
    }.withCLI("bundles", "Target bundle formats (e.g. deb, rpm, or deb,rpm)") {
        completeWith("deb", "rpm", "deb,rpm")
    }

    private fun executionPath(): String {
        val userHome = System.getProperty("user.home") ?: ""
        return listOf(
            "$userHome/.local/share/mise/shims",
            "$userHome/.cargo/bin",
            "$userHome/.local/bin",
            System.getenv("PATH") ?: ""
        ).filter { it.isNotBlank() }.joinToString(":")
    }

    override suspend fun buildInputs(context: ExecutionContext) = NoInputs()
    override fun buildOutputs() = NoOutputs()

    override suspend fun execute(context: ExecutionContext, inputSet: NoInputs): ExecutionResult<NoOutputs> {
        val targetBundles = bundles.orNull() ?: "deb"
        return context.executeBash {
            workingDir = context.projectDir.toString()
            environment("PATH", executionPath())
            command("bash")
            args(
                "-c",
                "pnpm --filter @kandev/desktop exec tauri build --features desktop-runtime --bundles $targetBundles"
            )
        }
    }
}

/**
 * Sets up the fork's agy-acp bridge:
 * Resolves repository, builds dist/main.js, and symlinks to ~/.local/bin/agy-acp.
 */
open class ForkAgyAcpTask(name: String) : Task<NoInputs, NoOutputs>(name) {
    private fun executionPath(): String {
        val userHome = System.getProperty("user.home") ?: ""
        return listOf(
            "$userHome/.local/share/mise/shims",
            "$userHome/.cargo/bin",
            "$userHome/.local/bin",
            System.getenv("PATH") ?: ""
        ).filter { it.isNotBlank() }.joinToString(":")
    }

    override suspend fun buildInputs(context: ExecutionContext) = NoInputs()
    override fun buildOutputs() = NoOutputs()

    override suspend fun execute(context: ExecutionContext, inputSet: NoInputs): ExecutionResult<NoOutputs> {
        val userHome = System.getProperty("user.home") ?: ""
        val candidateDirs = listOf(
            File(context.projectDir.toFile().parentFile, "agy-acp"),
            File(userHome, "MiscProjects/agy-acp")
        )
        val agyDir = candidateDirs.firstOrNull { it.exists() && it.isDirectory }
            ?: File(userHome, "MiscProjects/agy-acp")

        val script = """
            set -euo pipefail
            AGY_DIR="${agyDir.absolutePath}"
            if [ ! -d "${'$'}AGY_DIR" ]; then
                echo "Cloning BrunoSilvaFreire/agy-acp to ${'$'}AGY_DIR..."
                git clone https://github.com/BrunoSilvaFreire/agy-acp.git "${'$'}AGY_DIR"
            fi
            cd "${'$'}AGY_DIR"
            if [ ! -d "node_modules" ]; then
                echo "Installing agy-acp dependencies..."
                pnpm install --frozen-lockfile
            fi
            echo "Building agy-acp..."
            pnpm run build
            chmod +x dist/main.js

            mkdir -p "$userHome/.local/bin"
            ln -sf "${'$'}AGY_DIR/dist/main.js" "$userHome/.local/bin/agy-acp"
            echo "agy-acp linked to $userHome/.local/bin/agy-acp"

            # Verify agy-acp responds to initialize
            echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1}}' | "$userHome/.local/bin/agy-acp" >/dev/null
            echo "agy-acp probe successful."
        """.trimIndent()

        return context.executeBash {
            environment("PATH", executionPath())
            command("bash")
            args("-c", script)
        }
    }
}

/**
 * Installs the built .deb package locally via apt-get with elevation,
 * and sets up system-wide fork binaries (/usr/local/bin/agy-acp).
 */
open class DesktopInstallTask(name: String) : Task<NoInputs, NoOutputs>(name) {
    override suspend fun buildInputs(context: ExecutionContext) = NoInputs()
    override fun buildOutputs() = NoOutputs()

    override suspend fun execute(context: ExecutionContext, inputSet: NoInputs): ExecutionResult<NoOutputs> {
        val debDir = File(context.projectDir.toFile(), "apps/desktop/src-tauri/target/release/bundle/deb")
        if (!debDir.exists() || !debDir.isDirectory) {
            return failure(
                IllegalStateException(
                    "Deb bundle directory does not exist: ${debDir.path}. " +
                    "Run 'karto run desktop/build' first."
                )
            )
        }

        val debFiles = debDir.listFiles { _, fileName -> fileName.endsWith(".deb") }
            ?.sortedByDescending { it.lastModified() }
            ?: emptyList()

        if (debFiles.isEmpty()) {
            return failure(
                IllegalStateException(
                    "No .deb package found in ${debDir.path}. " +
                    "Run 'karto run desktop/build' first."
                )
            )
        }

        val targetDeb = debFiles.first()
        val userHome = System.getProperty("user.home") ?: ""
        val agyAcpUserBin = File(userHome, ".local/bin/agy-acp")

        val installScript = """
            set -euo pipefail
            echo "Installing deb package: ${targetDeb.absolutePath}..."
            apt-get install -y "${targetDeb.absolutePath}"

            if [ -e "${agyAcpUserBin.absolutePath}" ]; then
                target="${'$'}(readlink -f "${agyAcpUserBin.absolutePath}" 2>/dev/null || echo "${agyAcpUserBin.absolutePath}")"
                if [ -n "${'$'}target" ] && [ -f "${'$'}target" ]; then
                    echo "Linking agy-acp system-wide to /usr/local/bin/agy-acp..."
                    ln -sf "${'$'}target" /usr/local/bin/agy-acp
                    chmod +x /usr/local/bin/agy-acp
                fi
            fi
            echo "Installation and fork setup complete."
        """.trimIndent()

        return context.executeBash {
            requiresElevation = true
            command("bash")
            args("-c", installScript)
        }
    }
}

/**
 * Cleans desktop build outputs and staged runtime resources.
 */
open class DesktopCleanTask(name: String) : Task<NoInputs, NoOutputs>(name) {
    override suspend fun buildInputs(context: ExecutionContext) = NoInputs()
    override fun buildOutputs() = NoOutputs()

    override suspend fun execute(context: ExecutionContext, inputSet: NoInputs): ExecutionResult<NoOutputs> {
        val baseDir = context.projectDir.toFile()
        val bundleDir = File(baseDir, "apps/desktop/src-tauri/target/release/bundle")
        val runtimeResourcesDir = File(baseDir, "apps/desktop/src-tauri/resources/kandev/bin")

        if (bundleDir.exists()) {
            bundleDir.deleteRecursively()
        }
        if (runtimeResourcesDir.exists()) {
            runtimeResourcesDir.deleteRecursively()
        }

        return success()
    }
}

/**
 * Launches KanDev Desktop in profiling mode (KANDEV_PROFILE=1).
 * Creates a timestamped session in .profile/<session-id>/, records process manifest,
 * exposes loopback pprof, permits DevTools inspection, and keeps attached for manual interaction.
 */
open class ProfilingDesktopTask(name: String) : Task<NoInputs, NoOutputs>(name) {
    val perf = property<Boolean> {
        set(false)
    }.withCLI("perf", "Record session under Linux perf if permitted")

    private fun executionPath(): String {
        val userHome = System.getProperty("user.home") ?: ""
        return listOf(
            "$userHome/.local/share/mise/shims",
            "$userHome/.cargo/bin",
            "$userHome/.local/bin",
            System.getenv("PATH") ?: ""
        ).filter { it.isNotBlank() }.joinToString(":")
    }

    override suspend fun buildInputs(context: ExecutionContext) = NoInputs()
    override fun buildOutputs() = NoOutputs()

    override suspend fun execute(context: ExecutionContext, inputSet: NoInputs): ExecutionResult<NoOutputs> {
        val perfFlag = if (perf.orNull() == true) "--perf" else ""
        return context.executeBash {
            workingDir = context.projectDir.toString()
            environment("PATH", executionPath())
            command("bash")
            args("scripts/profiling/launch-desktop-profile.sh", perfFlag)
        }
    }
}

/**
 * Captures a Go CPU profile from the active profiling session.
 */
open class ProfilingBackendCpuTask(name: String) : Task<NoInputs, NoOutputs>(name) {
    val seconds = property<Int> {
        set(30)
    }.withCLI("seconds", "Duration of CPU profile capture in seconds (default 30)")

    override suspend fun buildInputs(context: ExecutionContext) = NoInputs()
    override fun buildOutputs() = NoOutputs()

    override suspend fun execute(context: ExecutionContext, inputSet: NoInputs): ExecutionResult<NoOutputs> {
        val dur = seconds.orNull() ?: 30
        return context.executeBash {
            workingDir = context.projectDir.toString()
            command("bash")
            args("scripts/profiling/capture-pprof.sh", "cpu", dur.toString())
        }
    }
}

/**
 * Captures a Go Heap memory profile from the active profiling session.
 */
open class ProfilingBackendHeapTask(name: String) : Task<NoInputs, NoOutputs>(name) {
    override suspend fun buildInputs(context: ExecutionContext) = NoInputs()
    override fun buildOutputs() = NoOutputs()

    override suspend fun execute(context: ExecutionContext, inputSet: NoInputs): ExecutionResult<NoOutputs> {
        return context.executeBash {
            workingDir = context.projectDir.toString()
            command("bash")
            args("scripts/profiling/capture-pprof.sh", "heap")
        }
    }
}

/**
 * Summarizes the captured profile session and generates report.md.
 */
open class ProfilingReportTask(name: String) : Task<NoInputs, NoOutputs>(name) {
    override suspend fun buildInputs(context: ExecutionContext) = NoInputs()
    override fun buildOutputs() = NoOutputs()

    override suspend fun execute(context: ExecutionContext, inputSet: NoInputs): ExecutionResult<NoOutputs> {
        return context.executeBash {
            workingDir = context.projectDir.toString()
            command("bash")
            args("scripts/profiling/generate-report.sh")
        }
    }
}

/**
 * Cleans .profile/ output directory only. Does not touch build outputs.
 */
open class ProfilingCleanTask(name: String) : Task<NoInputs, NoOutputs>(name) {
    override suspend fun buildInputs(context: ExecutionContext) = NoInputs()
    override fun buildOutputs() = NoOutputs()

    override suspend fun execute(context: ExecutionContext, inputSet: NoInputs): ExecutionResult<NoOutputs> {
        val profileDir = File(context.projectDir.toFile(), ".profile")
        if (profileDir.exists()) {
            profileDir.deleteRecursively()
        }
        return success()
    }
}

/**
 * Configures the Debian host environment with elevation:
 * 1. Ensures 127.0.0.1 kandev.local is mapped in /etc/hosts.
 * 2. Configures net.ipv4.ip_unprivileged_port_start=80 in /etc/sysctl.d/99-kandev.conf and applies it.
 * 3. Applies CAP_NET_BIND_SERVICE capability to the launcher binary and links it to /usr/local/bin/kandev.
 */
open class ServiceSetupHostTask(name: String) : Task<NoInputs, NoOutputs>(name) {
    override suspend fun buildInputs(context: ExecutionContext) = NoInputs()
    override fun buildOutputs() = NoOutputs()

    override suspend fun execute(context: ExecutionContext, inputSet: NoInputs): ExecutionResult<NoOutputs> {
        val projectDir = context.projectDir.toFile()
        val candidateBins = listOf(
            File(projectDir, "dist/kandev/bin/kandev"),
            File("/usr/lib/Kandev/kandev/bin/kandev"),
            File(projectDir, "apps/desktop/src-tauri/resources/kandev/bin/kandev")
        )
        val launcherBin = candidateBins.firstOrNull { it.exists() }
        val launcherPath = launcherBin?.absolutePath ?: ""

        // Check if host environment is already configured
        val hostsFile = File("/etc/hosts")
        val hostsConfigured = hostsFile.exists() && hostsFile.readLines().any { line ->
            val trimmed = line.trim()
            !trimmed.startsWith("#") && trimmed.contains("127.0.0.1") && trimmed.contains("kandev.local")
        }

        val portFile = File("/proc/sys/net/ipv4/ip_unprivileged_port_start")
        val portStart = portFile.takeIf { it.exists() }?.readText()?.trim()?.toIntOrNull() ?: 1024
        val portConfigured = portStart <= 80

        val binSymlink = File("/usr/local/bin/kandev")
        val binConfigured = binSymlink.exists()

        if (hostsConfigured && portConfigured && binConfigured) {
            println("Host environment already configured for kandev.local on port 80.")
            return success()
        }

        val setupScript = """
            set -euo pipefail

            # 1. /etc/hosts mapping
            if ! grep -qE '^\s*127\.0\.0\.1\s+.*kandev\.local' /etc/hosts; then
                echo "Adding 127.0.0.1 kandev.local to /etc/hosts..."
                printf "\n127.0.0.1 kandev.local\n" >> /etc/hosts
            else
                echo "kandev.local is already present in /etc/hosts."
            fi

            # 2. Allow unprivileged port 80 binding
            echo "Configuring net.ipv4.ip_unprivileged_port_start=80..."
            echo "net.ipv4.ip_unprivileged_port_start=80" > /etc/sysctl.d/99-kandev.conf
            sysctl -w net.ipv4.ip_unprivileged_port_start=80 >/dev/null

            # 3. Setcap & symlink if launcher binary exists
            if [ -n "$launcherPath" ] && [ -f "$launcherPath" ]; then
                if command -v setcap >/dev/null 2>&1; then
                    echo "Setting CAP_NET_BIND_SERVICE on $launcherPath..."
                    setcap 'cap_net_bind_service=+ep' "$launcherPath" 2>/dev/null || true
                elif [ -x /sbin/setcap ]; then
                    echo "Setting CAP_NET_BIND_SERVICE on $launcherPath..."
                    /sbin/setcap 'cap_net_bind_service=+ep' "$launcherPath" 2>/dev/null || true
                fi

                echo "Linking $launcherPath to /usr/local/bin/kandev..."
                ln -sf "$launcherPath" /usr/local/bin/kandev
                chmod +x /usr/local/bin/kandev
            fi

            echo "Host setup complete: kandev.local mapped to 127.0.0.1."
        """.trimIndent()

        return context.executeBash {
            requiresElevation = true
            command("bash")
            args("-c", setupScript)
        }
    }
}

/**
 * Installs the user-level systemd service using the native KanDev launcher
 * with a dedicated home directory (~/.kandev-service) and port 80.
 */
open class ServiceInstallTask(name: String) : Task<NoInputs, NoOutputs>(name) {
    private fun nvmBinPath(): String {
        val userHome = System.getProperty("user.home") ?: ""
        return System.getenv("NVM_BIN") ?: run {
            val nodeDir = File(userHome, ".nvm/versions/node")
            if (nodeDir.exists() && nodeDir.isDirectory) {
                nodeDir.listFiles()?.filter { it.isDirectory }?.maxByOrNull { it.lastModified() }?.let {
                    File(it, "bin").absolutePath
                }
            } else null
        } ?: ""
    }

    private fun executionPath(): String {
        val userHome = System.getProperty("user.home") ?: ""
        return listOf(
            nvmBinPath(),
            "$userHome/.local/share/mise/shims",
            "$userHome/.cargo/bin",
            "$userHome/.local/bin",
            System.getenv("PATH") ?: ""
        ).filter { it.isNotBlank() }.joinToString(":")
    }

    override suspend fun buildInputs(context: ExecutionContext) = NoInputs()
    override fun buildOutputs() = NoOutputs()

    override suspend fun execute(context: ExecutionContext, inputSet: NoInputs): ExecutionResult<NoOutputs> {
        val userHome = System.getProperty("user.home") ?: ""
        val serviceHome = File(userHome, ".kandev-service")
        val projectDir = context.projectDir.toFile()
        val candidateBins = listOf(
            File(projectDir, "dist/kandev/bin/kandev"),
            File("/usr/lib/Kandev/kandev/bin/kandev"),
            File(projectDir, "apps/desktop/src-tauri/resources/kandev/bin/kandev")
        )
        val launcherBin = candidateBins.firstOrNull { it.exists() }
            ?: return failure(IllegalStateException(
                "KanDev launcher binary not found. Run 'karto run build/runtime' first."
            ))

        val nvmBin = nvmBinPath()
        val installScript = """
            set -euo pipefail
            LAUNCHER="${launcherBin.absolutePath}"
            SERVICE_HOME="${serviceHome.absolutePath}"
            mkdir -p "${'$'}SERVICE_HOME"

            if [ -n "$nvmBin" ]; then
                export NVM_BIN="$nvmBin"
            fi

            echo "Installing KanDev systemd user service via '${'$'}LAUNCHER'..."
            "${'$'}LAUNCHER" service install --port 80 --home-dir "${'$'}SERVICE_HOME"

            echo "Enabling and starting kandev.service..."
            systemctl --user daemon-reload
            systemctl --user enable --now kandev.service

            if systemctl --user is-active --quiet kandev.service; then
                echo "KanDev service is running!"
                echo "Access KanDev in your browser at: http://kandev.local/"
            else
                echo "WARNING: kandev.service was installed but is not active yet. Check 'karto run service/status' or 'karto run service/logs'."
            fi
        """.trimIndent()

        return context.executeBash {
            environment("PATH", executionPath())
            command("bash")
            args("-c", installScript)
        }
    }
}

/**
 * Manages the user-level systemd service lifecycle: start, stop, restart, status, uninstall.
 */
open class ServiceControlTask(name: String) : Task<NoInputs, NoOutputs>(name) {
    var action: String = "status"

    override suspend fun buildInputs(context: ExecutionContext) = NoInputs()
    override fun buildOutputs() = NoOutputs()

    override suspend fun execute(context: ExecutionContext, inputSet: NoInputs): ExecutionResult<NoOutputs> {
        val script = when (action) {
            "start" -> """
                systemctl --user start kandev.service
                echo "KanDev service started."
                systemctl --user is-active kandev.service || true
            """.trimIndent()
            "stop" -> """
                systemctl --user stop kandev.service
                echo "KanDev service stopped."
            """.trimIndent()
            "restart" -> """
                systemctl --user restart kandev.service
                echo "KanDev service restarted."
                systemctl --user is-active kandev.service || true
            """.trimIndent()
            "status" -> """
                systemctl --user status kandev.service --no-pager || true
            """.trimIndent()
            "uninstall" -> """
                echo "Stopping and disabling kandev.service..."
                systemctl --user disable --now kandev.service 2>/dev/null || true
                rm -f "${'$'}HOME/.config/systemd/user/kandev.service"
                systemctl --user daemon-reload
                systemctl --user reset-failed kandev.service 2>/dev/null || true
                echo "KanDev service uninstalled."
            """.trimIndent()
            else -> return failure(IllegalArgumentException("Unknown service action: $action"))
        }

        return context.executeBash {
            command("bash")
            args("-c", script)
        }
    }
}

/**
 * Inspects systemd logs for the KanDev service via journalctl.
 */
open class ServiceLogsTask(name: String) : Task<NoInputs, NoOutputs>(name) {
    val follow = property<Boolean> {
        set(false)
    }.withCLI("follow", "Follow logs in real time")

    override suspend fun buildInputs(context: ExecutionContext) = NoInputs()
    override fun buildOutputs() = NoOutputs()

    override suspend fun execute(context: ExecutionContext, inputSet: NoInputs): ExecutionResult<NoOutputs> {
        val followFlag = if (follow.orNull() == true) "-f" else ""
        return context.executeBash {
            command("bash")
            args("-c", "journalctl --user-unit kandev.service -n 200 --no-pager $followFlag")
        }
    }
}

/**
 * Safely transfers KanDev persistent state between desktop (~/.kandev) and service (~/.kandev-service).
 * 
 * Invariants:
 * 1. Checks that the active source/destination database is not locked by a live process.
 * 2. Stops the service runtime if active before data transfer to prevent concurrent SQLite writes.
 * 3. Creates a timestamped destination backup before overwriting.
 * 4. Copies only persistent state (data, sessions, quick-chat, tasks, skills, plugins, tools).
 * 5. Excludes temporary state (.kandev-backend.lock, logs, cache, supervisor, tmp).
 * 6. Restarts the service runtime if it was previously active.
 */
open class DataTransferTask(name: String) : Task<NoInputs, NoOutputs>(name) {
    var toService: Boolean = true

    override suspend fun buildInputs(context: ExecutionContext) = NoInputs()
    override fun buildOutputs() = NoOutputs()

    override suspend fun execute(context: ExecutionContext, inputSet: NoInputs): ExecutionResult<NoOutputs> {
        val userHome = System.getProperty("user.home") ?: ""
        val desktopDir = File(userHome, ".kandev")
        val serviceDir = File(userHome, ".kandev-service")
        val (srcDir, dstDir) = if (toService) (desktopDir to serviceDir) else (serviceDir to desktopDir)
        val directionLabel = if (toService) "desktop -> service" else "service -> desktop"

        println("=== KanDev Data Transfer: $directionLabel ===")
        println("Source     : ${srcDir.absolutePath}")
        println("Destination: ${dstDir.absolutePath}")

        if (!srcDir.exists() || !srcDir.isDirectory) {
            return failure(IllegalStateException("Source directory ${srcDir.absolutePath} does not exist."))
        }

        // Safety Guard 1: Verify source is not actively running
        val srcLockedPid = getLockedPid(srcDir)
        if (srcLockedPid != null) {
            return failure(IllegalStateException(
                "Cannot transfer data ($directionLabel): source runtime is actively running (PID $srcLockedPid). " +
                "Please close the application first to avoid database corruption."
            ))
        }

        // Safety Guard 2: If desktop is destination, verify it is not running
        if (!toService) {
            val desktopLockedPid = getLockedPid(desktopDir)
            if (desktopLockedPid != null || isDesktopProcessRunning()) {
                return failure(IllegalStateException(
                    "Cannot transfer data ($directionLabel): desktop runtime is actively running. " +
                    "Please quit KanDev Desktop before replacing its state."
                ))
            }
        }

        // Safety Guard 3: Stop service if active before copy
        val serviceWasActive = isServiceActive()
        if (serviceWasActive) {
            println("Stopping kandev.service before data transfer...")
            val stopExit = ProcessBuilder("systemctl", "--user", "stop", "kandev.service").start().waitFor()
            if (stopExit != 0) {
                return failure(IllegalStateException("Failed to stop kandev.service before data transfer."))
            }
            println("kandev.service stopped cleanly.")
        }

        // Safety Guard 4: Backup destination before replacing
        val durableDirs = listOf("data", "sessions", "quick-chat", "tasks", "skills", "plugins", "tools")
        val existingDurable = durableDirs.filter { File(dstDir, it).exists() }
        if (existingDurable.isNotEmpty()) {
            val timestamp = DateTimeFormatter.ofPattern("yyyyMMdd_HHmmss").format(LocalDateTime.now())
            val backupDir = File(dstDir.parentFile, "${dstDir.name}.backup-$timestamp")
            println("Backing up existing ${dstDir.name} to ${backupDir.absolutePath}...")
            backupDir.mkdirs()
            for (dName in existingDurable) {
                val src = File(dstDir, dName)
                val dst = File(backupDir, dName)
                copyTree(src, dst)
            }
            println("Backup completed successfully: ${backupDir.absolutePath}")
        }

        // Transfer durable state
        println("Copying persistent state...")
        dstDir.mkdirs()
        var copiedCount = 0
        for (dName in durableDirs) {
            val src = File(srcDir, dName)
            val dst = File(dstDir, dName)
            if (src.exists()) {
                if (dst.exists()) {
                    dst.deleteRecursively()
                }
                copyTree(src, dst)
                println("  ✓ Transferred $dName/")
                copiedCount++
            }
        }

        // Remove stale lock files in destination
        File(dstDir, ".kandev-backend.lock").delete()

        // Restart service if needed
        if (serviceWasActive || toService) {
            println("Starting kandev.service...")
            ProcessBuilder("systemctl", "--user", "start", "kandev.service").start().waitFor()
            if (isServiceActive()) {
                println("kandev.service is running and accessible at http://kandev.local/")
            } else {
                println("WARNING: kandev.service was started but is not active yet.")
            }
        }

        println("Data transfer complete ($directionLabel). $copiedCount directories transferred.")
        return success()
    }

    private fun copyTree(source: File, target: File) {
        val srcPath = source.toPath()
        val dstPath = target.toPath()
        Files.walk(srcPath).forEach { path ->
            val rel = srcPath.relativize(path)
            val dest = dstPath.resolve(rel)
            if (Files.isSymbolicLink(path)) {
                Files.deleteIfExists(dest)
                val linkTarget = Files.readSymbolicLink(path)
                try {
                    dest.parent?.let { Files.createDirectories(it) }
                    Files.createSymbolicLink(dest, linkTarget)
                } catch (_: Exception) {
                    // Ignore broken/unsupported link creation
                }
            } else if (Files.isDirectory(path)) {
                Files.createDirectories(dest)
            } else {
                dest.parent?.let { Files.createDirectories(it) }
                Files.copy(path, dest, java.nio.file.StandardCopyOption.REPLACE_EXISTING)
            }
        }
    }

    private fun getLockedPid(dir: File): Long? {
        val lockFile = File(dir, ".kandev-backend.lock")
        if (!lockFile.exists()) return null
        return try {
            val content = lockFile.readText()
            val match = Regex(""""pid"\s*:\s*(\d+)""").find(content)
            val pid = match?.groupValues?.get(1)?.toLongOrNull()
            if (pid != null && isProcessAlive(pid)) pid else null
        } catch (_: Exception) {
            null
        }
    }

    private fun isProcessAlive(pid: Long): Boolean {
        return try {
            ProcessHandle.of(pid).map { it.isAlive }.orElse(false)
        } catch (_: Exception) {
            false
        }
    }

    private fun isDesktopProcessRunning(): Boolean {
        return try {
            val p = ProcessBuilder("pgrep", "-f", "kandev-desktop").start()
            p.waitFor() == 0
        } catch (_: Exception) {
            false
        }
    }

    private fun isServiceActive(): Boolean {
        return try {
            val p = ProcessBuilder("systemctl", "--user", "is-active", "--quiet", "kandev.service").start()
            p.waitFor() == 0
        } catch (_: Exception) {
            false
        }
    }
}

// ─── Fork Pipeline ──────────────────────────────────────────────
group("fork") {
    val agyAcp = task<ForkAgyAcpTask>("agy-acp") {}
    val setup = task<ForkAgyAcpTask>("setup") {}
}

// ─── Shared Build Pipeline ──────────────────────────────────────
group("build") {
    val web = task<WebBuildTask>("web") {}

    val backend = task<BackendBuildTask>("backend") {}

    val runtime = task<RuntimeBundleTask>("runtime") {}

    // The runtime bundle already rebuilds web and backend, so keep the standalone
    // compilation tasks out of the group default: `karto run build` packages once.
    excludeTasksFromGroupDefault(web, backend)
}

// ─── Desktop Pipeline ───────────────────────────────────────────
group("desktop") {
    val runtime = task<DesktopStageRuntimeTask>("runtime") {
        dependsOn("build/runtime")
    }

    val verify = task<DesktopVerifyTask>("verify") {
        dependsOn(runtime)
    }

    val build = task<DesktopBuildTask>("build") {
        dependsOn(verify)
    }

    val forkSetup = task<ForkAgyAcpTask>("fork-setup") {}

    val install = task<DesktopInstallTask>("install") {
        dependsOn(build, forkSetup)
    }

    val aptInstall = task<DesktopInstallTask>("aptInstall") {
        dependsOn(build, forkSetup)
    }

    val all = group("all") {
        group.dependsOn(runtime, verify, build, forkSetup, install)
    }

    val clean = task<DesktopCleanTask>("clean") {}

    excludeTasksFromGroupDefault(clean, all.group, forkSetup, aptInstall)
}

// ─── Profiling Pipeline ─────────────────────────────────────────
group("profiling") {
    val build = task<ProfilingBuildTask>("build") {}

    val desktop = task<ProfilingDesktopTask>("desktop") {
        dependsOn(build)
    }

    val taskPage = task<ProfilingDesktopTask>("task-page") {
        dependsOn(build)
    }

    val backendCpu = task<ProfilingBackendCpuTask>("backend-cpu") {}

    val backendHeap = task<ProfilingBackendHeapTask>("backend-heap") {}

    val report = task<ProfilingReportTask>("report") {}

    val clean = task<ProfilingCleanTask>("clean") {}

    excludeTasksFromGroupDefault(clean, backendCpu, backendHeap, report, taskPage)
}

/**
 * Writes the systemd user-service feature-flag drop-in and reloads the manager.
 *
 * `kandev service install` regenerates kandev.service from a fixed template and
 * would discard any flags edited into it, so runtime feature flags live in a
 * drop-in instead. Both service/install and service/restart depend on this task,
 * guaranteeing the flags are present before the service (re)starts.
 */
open class ServiceFeatureFlagsTask(name: String) : Task<NoInputs, NoOutputs>(name) {
    override suspend fun buildInputs(context: ExecutionContext) = NoInputs()
    override fun buildOutputs() = NoOutputs()

    override suspend fun execute(context: ExecutionContext, inputSet: NoInputs): ExecutionResult<NoOutputs> {
        val userHome = System.getProperty("user.home") ?: ""
        val dropInDir = File(userHome, ".config/systemd/user/kandev.service.d")
        val dropInFile = File(dropInDir, "10-kandev-flags.conf")

        val contents = """
            # managed by karto — runtime feature flags for kandev.service
            # Regenerated by the service/install and service/restart karto tasks.
            [Service]
            Environment=KANDEV_FEATURES_DYNAMIC_AGENT_ROUTING=true
        """.trimIndent() + "\n"

        dropInDir.mkdirs()
        if (!dropInFile.exists() || dropInFile.readText() != contents) {
            dropInFile.writeText(contents)
            println("Wrote feature-flag drop-in: ${dropInFile.absolutePath}")
        } else {
            println("Feature-flag drop-in already current: ${dropInFile.absolutePath}")
        }

        return context.executeBash {
            command("bash")
            args("-c", "systemctl --user daemon-reload && echo 'Reloaded systemd user manager.'")
        }
    }
}

// ─── Service Pipeline ───────────────────────────────────────────
group("service") {
    // The service runs the packaged runtime bundle directly from dist/kandev; it does
    // not need the desktop staging step, so it depends on the shared build instead.
    val setupHost = task<ServiceSetupHostTask>("setup-host") {
        dependsOn("build/runtime")
    }

    // Runtime feature flags are applied as a systemd drop-in so they survive the
    // unit regeneration performed by `kandev service install`.
    val flags = task<ServiceFeatureFlagsTask>("flags") {}

    val install = task<ServiceInstallTask>("install") {
        dependsOn(setupHost, flags)
    }

    val start = task<ServiceControlTask>("start") {
        action = "start"
    }

    val stop = task<ServiceControlTask>("stop") {
        action = "stop"
    }

    val restart = task<ServiceControlTask>("restart") {
        action = "restart"
        dependsOn(flags)
    }

    val status = task<ServiceControlTask>("status") {
        action = "status"
    }

    val logs = task<ServiceLogsTask>("logs") {}

    val uninstall = task<ServiceControlTask>("uninstall") {
        action = "uninstall"
    }

    excludeTasksFromGroupDefault(setupHost, flags, stop, restart, logs, uninstall)
}

// ─── Data Pipeline ──────────────────────────────────────────────
group("data") {
    val desktopToService = task<DataTransferTask>("desktop-to-service") {
        toService = true
    }

    val serviceToDesktop = task<DataTransferTask>("service-to-desktop") {
        toService = false
    }

    excludeTasksFromGroupDefault(serviceToDesktop)
}
