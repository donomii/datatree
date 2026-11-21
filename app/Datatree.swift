// app/Datatree.swift
// Datatree.app: runs the bundled datatree executable as a normal macOS app.
// Launch starts the server (which opens the browser), clicking the Dock icon
// or File > Open in Browser opens the browser again, and Quit stops the server.
import AppKit
import Foundation

let servingPrefix = "Datatree is running at "
let restartDelaySeconds = 2.0
let shellPathTimeoutSeconds = 5.0
let quitTimeoutSeconds = 5.0

func appendLog(_ handle: FileHandle, _ message: String) {
	let stamp = ISO8601DateFormatter().string(from: Date())
	handle.write(Data("\(stamp) Datatree.app: \(message)\n".utf8))
}

func fallbackPath() -> String {
	let home = NSHomeDirectory()
	return ["/opt/homebrew/bin", "/opt/homebrew/sbin", "/usr/local/bin", home + "/.local/bin", "/usr/bin", "/bin", "/usr/sbin", "/sbin"].joined(separator: ":")
}

// Apps started from Finder get a minimal PATH. Ask the login shell for the
// PATH Terminal uses, so gh, kubectl, aws and git resolve the same way.
func loginShellPath(log: FileHandle) -> String {
	let fallback = fallbackPath()
	guard let entry = getpwuid(getuid()), let shellPointer = entry.pointee.pw_shell else {
		appendLog(log, "cannot read the login shell from the user database; using PATH \(fallback)")
		return fallback
	}
	let shell = String(cString: shellPointer)
	let process = Process()
	process.executableURL = URL(fileURLWithPath: shell)
	process.arguments = ["-l", "-c", "printf '%s' \"$PATH\""]
	let output = Pipe()
	process.standardOutput = output
	process.standardError = FileHandle.nullDevice
	process.standardInput = FileHandle.nullDevice
	do {
		try process.run()
	} catch {
		appendLog(log, "cannot start login shell \(shell) to read PATH: \(error); using PATH \(fallback)")
		return fallback
	}
	let deadline = Date().addingTimeInterval(shellPathTimeoutSeconds)
	while process.isRunning && Date() < deadline {
		Thread.sleep(forTimeInterval: 0.05)
	}
	if process.isRunning {
		process.terminate()
		appendLog(log, "login shell \(shell) did not print PATH within \(shellPathTimeoutSeconds) s; using PATH \(fallback)")
		return fallback
	} else {
		let data = output.fileHandleForReading.readDataToEndOfFile()
		let path = String(decoding: data, as: UTF8.self).trimmingCharacters(in: .whitespacesAndNewlines)
		if process.terminationStatus == 0 && path.contains("/usr/bin") {
			appendLog(log, "using PATH from login shell \(shell): \(path)")
			return path
		} else {
			appendLog(log, "login shell \(shell) exited with status \(process.terminationStatus) and printed PATH \(path.debugDescription), expected status 0 and a PATH containing /usr/bin; using PATH \(fallback)")
			return fallback
		}
	}
}

func openLog() -> (FileHandle, String)? {
	let directory = NSHomeDirectory() + "/Library/Logs/Datatree"
	let path = directory + "/datatree.log"
	do {
		try FileManager.default.createDirectory(atPath: directory, withIntermediateDirectories: true)
	} catch {
		return nil
	}
	if FileManager.default.fileExists(atPath: path) {
		// Append to the existing log.
	} else {
		FileManager.default.createFile(atPath: path, contents: nil)
	}
	guard let handle = FileHandle(forWritingAtPath: path) else {
		return nil
	}
	handle.seekToEndOfFile()
	return (handle, path)
}

final class AppDelegate: NSObject, NSApplicationDelegate {
	var log: FileHandle = FileHandle.standardError
	var logPath = ""
	var executable = URL(fileURLWithPath: "/")
	var environment: [String: String] = [:]
	var server: Process?
	var serverURL: URL?
	var pending = ""
	var stopping = false

	func applicationDidFinishLaunching(_ notification: Notification) {
		buildMenu()
		guard let (handle, path) = openLog() else {
			failLaunch("cannot open the log file \(NSHomeDirectory())/Library/Logs/Datatree/datatree.log for writing")
			return
		}
		log = handle
		logPath = path
		guard let resources = Bundle.main.resourceURL else {
			failLaunch("cannot find the Resources directory of \(Bundle.main.bundlePath)")
			return
		}
		executable = resources.appendingPathComponent("datatree")
		var env = ProcessInfo.processInfo.environment
		env["PATH"] = loginShellPath(log: log)
		environment = env
		startServer()
	}

	func buildMenu() {
		let mainMenu = NSMenu()
		let appItem = NSMenuItem()
		let appMenu = NSMenu()
		appMenu.addItem(withTitle: "Open in Browser", action: #selector(openBrowser), keyEquivalent: "o")
		appMenu.addItem(withTitle: "Show Log", action: #selector(showLog), keyEquivalent: "l")
		appMenu.addItem(NSMenuItem.separator())
		appMenu.addItem(withTitle: "Quit Datatree", action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q")
		appItem.submenu = appMenu
		mainMenu.addItem(appItem)
		NSApp.mainMenu = mainMenu
	}

	func applicationDockMenu(_ sender: NSApplication) -> NSMenu? {
		let menu = NSMenu()
		menu.addItem(withTitle: "Open in Browser", action: #selector(openBrowser), keyEquivalent: "")
		return menu
	}

	func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows flag: Bool) -> Bool {
		openBrowser()
		return false
	}

	@objc func openBrowser() {
		if let url = serverURL {
			NSWorkspace.shared.open(url)
		} else {
			appendLog(log, "cannot open the browser: expected the server to have printed a line starting with \(servingPrefix.debugDescription), none seen yet")
			NSSound.beep()
		}
	}

	@objc func showLog() {
		if logPath.isEmpty {
			NSSound.beep()
		} else {
			NSWorkspace.shared.open(URL(fileURLWithPath: logPath))
		}
	}

	func startServer() {
		let process = Process()
		process.executableURL = executable
		process.arguments = []
		process.environment = environment
		process.currentDirectoryURL = URL(fileURLWithPath: NSHomeDirectory())
		let pipe = Pipe()
		process.standardOutput = pipe
		process.standardError = pipe
		process.standardInput = FileHandle.nullDevice
		pipe.fileHandleForReading.readabilityHandler = { [weak self] handle in
			let data = handle.availableData
			if data.isEmpty {
				handle.readabilityHandler = nil
			} else {
				DispatchQueue.main.async { self?.receive(data) }
			}
		}
		process.terminationHandler = { [weak self] finished in
			DispatchQueue.main.async { self?.serverExited(finished) }
		}
		do {
			try process.run()
			server = process
			appendLog(log, "started \(executable.path) as pid \(process.processIdentifier)")
		} catch {
			failLaunch("cannot start \(executable.path): \(error)")
		}
	}

	func receive(_ data: Data) {
		log.write(data)
		pending += String(decoding: data, as: UTF8.self)
		var lines = pending.components(separatedBy: "\n")
		pending = lines.removeLast()
		for line in lines {
			if let range = line.range(of: servingPrefix) {
				let text = line[range.upperBound...].trimmingCharacters(in: .whitespaces)
				serverURL = URL(string: text)
			} else {
				// Other server output only goes to the log.
			}
		}
	}

	func serverExited(_ finished: Process) {
		server = nil
		if stopping {
			appendLog(log, "server pid \(finished.processIdentifier) stopped with status \(finished.terminationStatus)")
			NSApp.reply(toApplicationShouldTerminate: true)
		} else {
			appendLog(log, "ERROR: server pid \(finished.processIdentifier) exited with status \(finished.terminationStatus) (reason \(finished.terminationReason.rawValue)), expected it to keep serving until Datatree.app quits; restarting in \(restartDelaySeconds) s")
			DispatchQueue.main.asyncAfter(deadline: .now() + restartDelaySeconds) { [weak self] in
				self?.startServer()
			}
		}
	}

	func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
		if let process = server, process.isRunning {
			stopping = true
			process.terminate()
			let pid = process.processIdentifier
			DispatchQueue.main.asyncAfter(deadline: .now() + quitTimeoutSeconds) { [weak self] in
				if let delegate = self, let current = delegate.server, current.isRunning {
					appendLog(delegate.log, "server pid \(pid) did not stop within \(quitTimeoutSeconds) s of SIGTERM; sending SIGKILL")
					kill(pid, SIGKILL)
				} else {
					// The server already stopped.
				}
			}
			return .terminateLater
		} else {
			return .terminateNow
		}
	}

	func failLaunch(_ message: String) {
		appendLog(log, "ERROR: \(message)")
		let alert = NSAlert()
		alert.messageText = "Datatree could not start"
		alert.informativeText = logPath.isEmpty ? message : message + "\n\nLog: " + logPath
		alert.runModal()
		NSApp.terminate(nil)
	}
}

let app = NSApplication.shared
let delegate = AppDelegate()
app.delegate = delegate
app.setActivationPolicy(.regular)
app.run()
