import SwiftUI
import UsageCore

struct UsageMetricView: View {
    let window: UsageWindow
    let style: UsageDisplayStyle
    let tint: Color
    let stale: Bool
    let now: Date

    private var expired: Bool { window.needsRefresh(at: now) }
    private var accent: Color {
        if stale || expired { return .secondary }
        if window.limited == true || (window.validPercent ?? 0) >= 95 { return .red }
        if (window.validPercent ?? 0) >= 85 { return .orange }
        return tint
    }
    private var value: String {
        window.validPercent.map { $0 >= 1_000 ? "999+%" : String(format: "%.0f%%", $0) }
            ?? (window.limited == true ? "Blocked" : "—")
    }
    private var reset: String {
        guard let date = window.resetsAt else { return "Reset ?" }
        if expired { return "Refresh" }
        return ResetPresentation.countdown(to: date, now: now).replacingOccurrences(of: "in ", with: "")
    }
    private var label: String {
        window.label == "month" ? "Monthly" : window.label == "budget" ? "Budget" : window.label
    }

    var body: some View {
        Group {
            if style == .gauges {
                HStack(spacing: 5) {
                    UsageGauge(window: window, tint: tint, stale: stale || expired, size: 34)
                    VStack(alignment: .leading, spacing: 3) {
                        Text(label).fontWeight(.medium).foregroundStyle(.primary)
                        Text(window.validPercent == nil && window.limited == true ? "Blocked" : reset)
                            .foregroundStyle(expired ? .orange : .secondary).monospacedDigit()
                    }
                    .font(.system(size: 10)).lineLimit(1)
                    .frame(maxWidth: .infinity, alignment: .leading)
                }
            } else {
                VStack(alignment: .leading, spacing: 3) {
                    HStack(spacing: 4) {
                        Text(label).foregroundStyle(.secondary)
                        Spacer(minLength: 0)
                        Text(value).fontWeight(.semibold).monospacedDigit().fixedSize()
                    }.font(.system(size: 11))
                    GeometryReader { geometry in
                        Capsule().fill(tint.opacity(0.12))
                            .overlay(alignment: .leading) {
                                Capsule().fill(accent).frame(
                                    width: geometry.size.width * min(1, max(0, (window.validPercent ?? 0) / 100)))
                            }
                    }.frame(height: 4).accessibilityHidden(true)
                    Text(expired ? "Refresh needed" : window.resetsAt == nil ? "Reset unknown" : "Reset \(reset)")
                        .font(.system(size: 10)).monospacedDigit()
                        .foregroundStyle(expired ? .orange : .secondary).lineLimit(1)
                }
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .help(
            window.resetsAt.map { "Resets \($0.formatted(date: .complete, time: .complete))" }
                ?? "Reset time not reported"
        )
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(
            "\(label), \(window.validPercent.map { String(format: "%.0f percent used", $0) } ?? (window.limited == true ? "blocked, percentage unavailable" : "usage unavailable")), \(expired ? "reset passed, refresh needed" : window.resetsAt == nil ? "reset not reported" : "reset in \(reset)")\(stale ? ", cached reading" : "")"
        )
    }
}
