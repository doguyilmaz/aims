import Foundation
import UsageCore

struct MenuSummary {
    static func value(snapshot: UsageSnapshot?, now: Date, globalError: String?) -> String? {
        guard globalError == nil else { return nil }
        let percent = snapshot?.providers.filter(\.installed).compactMap {
            DockSummary(provider: $0, now: now).percent
        }.max()
        return percent.map { $0 >= 1_000 ? "999%+" : String(format: "%.0f%%", $0) }
    }
}
