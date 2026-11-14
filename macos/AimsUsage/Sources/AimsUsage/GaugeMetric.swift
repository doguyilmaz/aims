import SwiftUI
import UsageCore

struct GaugeMetric: View {
    let window: UsageWindow
    let tint: Color
    let stale: Bool
    let now: Date

    private var expired: Bool { window.needsRefresh(at: now) }
    private var label: String {
        window.label == "month" ? "Monthly" : window.label == "budget" ? "Budget" : window.label
    }
    private var reset: String {
        guard let date = window.resetsAt else { return "Reset ?" }
        if expired { return "Refresh" }
        return ResetPresentation.countdown(to: date, now: now).replacingOccurrences(of: "in ", with: "")
    }

    var body: some View {
        HStack(spacing: 5) {
            UsageGauge(window: window, tint: tint, stale: stale || expired, size: 28)
            VStack(alignment: .leading, spacing: 1) {
                Text(label).font(.system(size: 11, weight: .medium)).lineLimit(1)
                Text(window.validPercent == nil && window.limited == true ? "Blocked" : reset)
                    .font(.system(size: 10)).monospacedDigit()
                    .foregroundStyle(expired ? .orange : .secondary).lineLimit(1)
                    .fixedSize(horizontal: true, vertical: false)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .help(
            window.resetsAt.map { "\(label) · Resets \($0.formatted(date: .complete, time: .complete))" }
                ?? "\(label) · Reset time not reported"
        )
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(
            "\(label), \(window.validPercent.map { String(format: "%.0f percent used", $0) } ?? (window.limited == true ? "blocked, percentage unavailable" : "usage unavailable")), \(expired ? "reset passed, refresh needed" : window.resetsAt == nil ? "reset not reported" : "reset in \(reset)")\(stale ? ", cached reading" : "")"
        )
    }
}
