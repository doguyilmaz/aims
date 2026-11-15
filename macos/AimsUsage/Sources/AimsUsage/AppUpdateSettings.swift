import SwiftUI

struct AppUpdateSettings: View {
    @ObservedObject var updater: AppUpdater

    var body: some View {
        if updater.isConfigured {
            Toggle(
                "Automatically check for app updates",
                isOn: Binding(
                    get: { updater.automaticChecks }, set: { updater.setAutomaticChecks($0) }))
            CheckForUpdatesButton(updater: updater)
            Text("Updates include the menu app, Edge Dock and bundled aims. Provider apps update separately.")
                .font(.caption).foregroundStyle(.secondary)
        } else {
            Text("This development build has no automatic update channel. Install a newer build manually.")
                .font(.caption).foregroundStyle(.secondary)
        }
    }
}
