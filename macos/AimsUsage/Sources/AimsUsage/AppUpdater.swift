import AppKit
import Combine
import Foundation
import Sparkle

@MainActor
final class AppUpdater: ObservableObject {
    @Published private(set) var canCheckForUpdates = false
    @Published private(set) var automaticChecks = false
    private let controller: SPUStandardUpdaterController?

    init(bundle: Bundle = .main) {
        guard bundle.object(forInfoDictionaryKey: "AimsUsagePreview") as? Bool != true,
            ProcessInfo.processInfo.environment["AIMS_USAGE_FIXTURE"] == nil,
            let feed = bundle.object(forInfoDictionaryKey: "SUFeedURL") as? String,
            let url = URL(string: feed), url.scheme == "https", url.host != nil,
            let key = bundle.object(forInfoDictionaryKey: "SUPublicEDKey") as? String,
            Data(base64Encoded: key)?.count == 32
        else {
            controller = nil
            return
        }
        let controller = SPUStandardUpdaterController(
            startingUpdater: false, updaterDelegate: nil, userDriverDelegate: nil)
        self.controller = controller
        controller.updater.publisher(for: \.canCheckForUpdates).assign(to: &$canCheckForUpdates)
        controller.updater.publisher(for: \.automaticallyChecksForUpdates).assign(to: &$automaticChecks)
    }

    var isConfigured: Bool { controller != nil }

    func start() { controller?.startUpdater() }

    func checkForUpdates() {
        guard canCheckForUpdates else { return }
        NSApp.activate(ignoringOtherApps: true)
        controller?.checkForUpdates(nil)
    }

    func setAutomaticChecks(_ enabled: Bool) {
        controller?.updater.automaticallyChecksForUpdates = enabled
    }
}
