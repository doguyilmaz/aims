import AppKit
import SwiftUI

struct UsageSettings: View {
    @Bindable var model: UsageModel

    var body: some View {
        Form {
            Section("Display") {
                Toggle("Show highest current usage in menu bar", isOn: $model.showMenuPercent)
                Toggle("Refresh providers every 5 minutes", isOn: $model.automaticRefresh)
                Text(
                    "Claude uses its cached native status line. Automatic refresh never sends a model prompt, buys credits or redeems resets."
                )
                .font(.caption).foregroundStyle(.secondary)
            }
            Section("aims executable") {
                TextField("Bundled aims (default)", text: $model.executablePath)
                    .textFieldStyle(.roundedBorder)
                HStack {
                    Button("Choose…") { chooseExecutable() }
                    Button("Use bundled") { model.executablePath = "" }
                }
                Text(
                    "Requires aims usage --json support. Provider logins remain in their existing homes; this app stores display preferences only."
                )
                .font(.caption).foregroundStyle(.secondary)
            }
            Section {
                Text(
                    "Aims Usage \(Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "0.1.0")"
                )
                .font(.caption).foregroundStyle(.secondary)
            }
        }.formStyle(.grouped).padding(12)
    }

    private func chooseExecutable() {
        let picker = NSOpenPanel()
        picker.canChooseFiles = true
        picker.canChooseDirectories = false
        picker.allowsMultipleSelection = false
        picker.prompt = "Choose aims"
        if picker.runModal() == .OK, let url = picker.url { model.executablePath = url.path }
    }
}
