import AppKit
import SwiftUI

struct UsageSettings: View {
    @Bindable var model: UsageModel
    @State private var configHome = ""
    @State private var additionalPaths = ""

    var body: some View {
        Form {
            Section("Display") {
                Picker("Widget appearance", selection: $model.displayStyle) {
                    ForEach(UsageDisplayStyle.allCases) { style in
                        Label(style.title, systemImage: style.symbol).tag(style)
                    }
                }
                Toggle("Show highest current usage in menu bar", isOn: $model.showMenuPercent)
                Toggle("Refresh providers every 5 minutes", isOn: $model.automaticRefresh)
                Text(
                    "Claude uses its cached native status line. Automatic refresh never sends a model prompt, buys credits or redeems resets."
                )
                .font(.caption).foregroundStyle(.secondary)
            }
            Section("Edge Dock") {
                Toggle("Show Edge Dock", isOn: $model.edgeDockEnabled)
                Picker("Screen edge", selection: $model.edgeDockSide) {
                    ForEach(EdgeDockSide.allCases) { Text($0.title).tag($0) }
                }
                Picker("Dock size", selection: $model.edgeDockSize) {
                    ForEach(EdgeDockSize.allCases) { Text($0.title).tag($0) }
                }
                VStack(alignment: .leading, spacing: 6) {
                    HStack {
                        Text("Vertical position")
                        Spacer()
                        Button("Center") { model.edgeDockPosition = 0.5 }
                            .controlSize(.small).accessibilityLabel("Center Edge Dock")
                    }
                    HStack {
                        Text("Top").font(.caption).foregroundStyle(.secondary)
                        Slider(value: $model.edgeDockPosition, in: 0...1)
                            .accessibilityLabel("Edge Dock vertical position")
                            .accessibilityValue("\(Int(model.edgeDockPosition * 100)) percent from top")
                        Text("Bottom").font(.caption).foregroundStyle(.secondary)
                    }
                }
                Toggle("Automatically hide", isOn: $model.edgeDockAutoHide)
                Text(
                    model.edgeDockAutoHide
                        ? "Hover over the edge marker to reveal the Dock. Pinned details keep it open."
                        : "The Dock stays visible. Provider details open toward the screen interior."
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
                    "Requires aims usage --json support. Provider logins remain in their existing homes; this app stores preferences and selected paths only."
                )
                .font(.caption).foregroundStyle(.secondary)
            }
            Section("aims data folder") {
                TextField("Default: ~/.aims", text: $configHome).textFieldStyle(.roundedBorder)
                HStack {
                    Button("Choose folder…") { chooseDataFolder() }
                    Button("Use default") { configHome = "" }
                }
                Text(
                    "For a custom AIMS_HOME, select the same existing folder you use in Terminal. Finder does not load shell exports. No files are moved."
                ).font(.caption).foregroundStyle(.secondary)
            }
            Section("Additional CLI folders") {
                TextEditor(text: $additionalPaths).font(.system(.caption, design: .monospaced)).frame(height: 68)
                Text(
                    "One absolute folder per line; ~/ is supported. These precede the standard Go, Cargo, npm, Homebrew and provider paths. No shell is started."
                ).font(.caption).foregroundStyle(.secondary)
                HStack {
                    Spacer()
                    Button("Apply paths") {
                        Task {
                            if await model.applyPaths(configHome: configHome, additionalPaths: additionalPaths) {
                                configHome = model.configHome
                                additionalPaths = model.additionalPaths
                            }
                        }
                    }
                    .disabled(
                        model.isRefreshing
                            || (configHome == model.configHome && additionalPaths == model.additionalPaths))
                }
                if let error = model.error { Text(error).font(.caption).foregroundStyle(.orange) }
            }
            Section {
                Text(
                    "Aims Usage \(Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "0.2.0") · \(Bundle.main.object(forInfoDictionaryKey: "AimsSourceRevision") as? String ?? "development")"
                )
                .font(.caption).foregroundStyle(.secondary)
            }
        }.formStyle(.grouped).padding(12)
            .onAppear {
                configHome = model.configHome
                additionalPaths = model.additionalPaths
            }
    }

    private func chooseExecutable() {
        let picker = NSOpenPanel()
        picker.canChooseFiles = true
        picker.canChooseDirectories = false
        picker.allowsMultipleSelection = false
        picker.prompt = "Choose aims"
        if picker.runModal() == .OK, let url = picker.url { model.executablePath = url.path }
    }

    private func chooseDataFolder() {
        let picker = NSOpenPanel()
        picker.canChooseFiles = false
        picker.canChooseDirectories = true
        picker.allowsMultipleSelection = false
        picker.prompt = "Choose aims data"
        if picker.runModal() == .OK, let url = picker.url { configHome = url.path }
    }
}
