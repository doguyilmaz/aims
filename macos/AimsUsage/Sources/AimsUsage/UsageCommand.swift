import AppKit
import Foundation
import UsageCore

// A single in-flight native process, owned by the UI task. Output goes to a
// private temporary file so a full pipe cannot deadlock termination.
@MainActor
final class UsageCommand {
    private var process: Process?
    private var continuation: CheckedContinuation<Data, any Error>?
    private var deadline: Task<Void, Never>?
    private var outputURL: URL?
    private var outputHandle: FileHandle?
    private var failure: (any Error)?
    private var shutdownContinuation: CheckedContinuation<Void, Never>?

    func cancelAndWait() async {
        guard process?.isRunning == true else { return }
        await withCheckedContinuation { continuation in
            shutdownContinuation = continuation
            stop(because: CancellationError())
        }
    }

    func run(executable: URL, arguments: [String], configHome: String = "", additionalPaths: String = "") async throws
        -> Data
    {
        try Task.checkCancellation()
        return try await withTaskCancellationHandler {
            try await withCheckedThrowingContinuation { continuation in
                self.continuation = continuation
                do {
                    try start(
                        executable: executable, arguments: arguments, configHome: configHome,
                        additionalPaths: additionalPaths)
                } catch { finish(.failure(error)) }
            }
        } onCancel: {
            Task { @MainActor in self.stop(because: CancellationError()) }
        }
    }

    private func start(executable: URL, arguments: [String], configHome: String, additionalPaths: String) throws {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent(
            UUID().uuidString, isDirectory: true)
        try FileManager.default.createDirectory(
            at: directory, withIntermediateDirectories: true, attributes: [.posixPermissions: 0o700])
        let output = directory.appendingPathComponent("usage.json")
        outputURL = output
        guard FileManager.default.createFile(atPath: output.path, contents: nil, attributes: [.posixPermissions: 0o600])
        else {
            throw CocoaError(.fileWriteUnknown)
        }
        let handle = try FileHandle(forWritingTo: output)
        outputHandle = handle
        let child = Process()
        child.executableURL = executable
        child.arguments = arguments
        child.currentDirectoryURL = directory
        let home = FileManager.default.homeDirectoryForCurrentUser.path
        child.environment = try UsageEnvironment.make(
            base: ProcessInfo.processInfo.environment, homeDirectory: home, aimsHome: configHome,
            additionalPaths: additionalPaths)
        child.standardOutput = handle
        child.standardError = FileHandle.nullDevice
        child.standardInput = FileHandle.nullDevice
        child.terminationHandler = { [weak self] process in
            let status = process.terminationStatus
            Task { @MainActor in self?.completed(status: status) }
        }
        process = child
        try child.run()
        deadline = Task { [weak self] in
            do { try await Task.sleep(for: .seconds(180)) } catch { return }
            self?.stop(because: UsageError.timedOut)
        }
    }

    private func stop(because error: any Error) {
        guard let child = process, child.isRunning else { return }
        failure = error
        child.terminate()  // aims usage cancels its provider contexts on SIGTERM.
        deadline?.cancel()
        deadline = Task { [weak self] in
            do { try await Task.sleep(for: .seconds(8)) } catch { return }
            guard let child = self?.process, child.isRunning else { return }
            kill(child.processIdentifier, SIGKILL)
        }
    }

    private func completed(status: Int32) {
        if let failure {
            finish(.failure(failure))
            return
        }
        guard status == 0 else {
            finish(.failure(UsageError.commandFailed(status)))
            return
        }
        do {
            try outputHandle?.close()
            guard let outputURL else { throw CocoaError(.fileReadUnknown) }
            let input = try FileHandle(forReadingFrom: outputURL)
            defer { try? input.close() }
            let data = try input.read(upToCount: 4_194_305) ?? Data()
            guard data.count <= 4_194_304 else { throw UsageError.outputTooLarge }
            finish(.success(data))
        } catch { finish(.failure(error)) }
    }

    private func finish(_ result: Result<Data, any Error>) {
        deadline?.cancel()
        deadline = nil
        try? outputHandle?.close()
        outputHandle = nil
        if let outputURL { try? FileManager.default.removeItem(at: outputURL.deletingLastPathComponent()) }
        outputURL = nil
        process = nil
        failure = nil
        let waiting = continuation
        continuation = nil
        waiting?.resume(with: result)
        shutdownContinuation?.resume()
        shutdownContinuation = nil
    }
}
