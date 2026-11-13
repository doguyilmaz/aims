import Foundation
import UsageCore

struct DockSummary {
    let percent: Double?
    let blocked: Bool

    init(provider: UsageProvider, now: Date, globalError: String? = nil) {
        let windows =
            provider.error == nil && globalError == nil
            ? provider.rows.filter { !$0.isStale(at: now) && $0.error == nil }
                .flatMap { $0.windows ?? [] }.filter { !$0.needsRefresh(at: now) }
            : []
        blocked = windows.contains { $0.limited == true }
        percent = windows.compactMap(\.validPercent).max()
    }

    var value: String {
        if blocked { return "Blocked" }
        return percent.map { $0 >= 1_000 ? "999+%" : String(format: "%.0f%%", $0) } ?? "—"
    }
}
