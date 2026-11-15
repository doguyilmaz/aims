import SwiftUI

struct TerminalCommandSettings: View {
    @State private var status: TerminalCommandStatus?
    @State private var message: String?
    @State private var busy = false
    @State private var installation: Task<Void, Never>?

    private var preview: Bool { Bundle.main.object(forInfoDictionaryKey: "AimsUsagePreview") as? Bool == true }

    var body: some View {
        Section("Terminal command") {
            if let path = status?.cliPath {
                Text("aims command found.")
                Text(path).font(.caption.monospaced()).foregroundStyle(.secondary)
                    .lineLimit(1).help(path).textSelection(.enabled)
                Text("Keep using its existing installer for updates. Its folder must be on your terminal shell PATH.")
                    .font(.caption).foregroundStyle(.secondary)
            } else {
                Text("The app includes aims. Add its command to Terminal without installing another copy.")
                    .font(.caption).foregroundStyle(.secondary)
                Button(busy ? "Checking…" : "Add aims to Terminal") {
                    installation = Task { await install() }
                }.disabled(busy || preview || status == nil)
                Text(
                    "Move the app to Applications first. The command is added to ~/.local/bin; existing CLI installations and shell files are kept."
                )
                .font(.caption).foregroundStyle(.secondary)
            }
            if let message { Text(message).font(.caption).foregroundStyle(.secondary).textSelection(.enabled) }
        }
        .task { await refresh() }
        .onDisappear { installation?.cancel() }
    }

    private func run(_ arguments: [String]) async throws -> Data {
        guard let executable = Bundle.main.url(forResource: "aims", withExtension: nil) else {
            throw CocoaError(.fileNoSuchFile)
        }
        return try await UsageCommand().run(executable: executable, arguments: ["app"] + arguments)
    }

    private func refresh() async {
        guard !busy else { return }
        busy = true
        defer { busy = false }
        do {
            status = try JSONDecoder().decode(TerminalCommandStatus.self, from: await run(["status", "--json"]))
        } catch is CancellationError {} catch {
            message = "Could not inspect the terminal command. Try aims app status in Terminal."
        }
    }

    private func install() async {
        guard !busy, !preview else { return }
        busy = true
        message = nil
        do {
            let result = try JSONDecoder().decode([String: String].self, from: await run(["link-cli", "--json"]))
            status = try JSONDecoder().decode(TerminalCommandStatus.self, from: await run(["status", "--json"]))
            message =
                "Terminal command: \(result["path"] ?? "aims"). Its folder must be on your shell PATH. Existing installations and shell files were kept."
        } catch is CancellationError {
        } catch {
            message =
                "Move the app to Applications and retry. Existing commands are kept; aims app link-cli in Terminal shows details."
        }
        busy = false
    }
}
