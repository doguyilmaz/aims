import SwiftUI
import UsageCore

struct UsageGauge: View {
    let window: UsageWindow?
    let tint: Color
    let stale: Bool
    let size: CGFloat

    private var percent: Double? { window?.validPercent }
    private var accent: Color {
        if stale { return tint.opacity(0.5) }
        if window?.limited == true || (percent ?? 0) >= 95 { return .red }
        if (percent ?? 0) >= 85 { return .orange }
        return tint
    }
    private var value: String {
        guard let percent else { return window?.limited == true ? "!" : "—" }
        return percent >= 1_000 ? "999+" : String(format: "%.0f", percent)
    }
    var body: some View {
        ZStack {
            Circle()
                .stroke(accent.opacity(0.13), style: StrokeStyle(lineWidth: size * 0.085, lineCap: .round))
            Circle().trim(from: 0, to: min(1, max(0, (percent ?? 0) / 100)))
                .stroke(
                    accent,
                    style: StrokeStyle(lineWidth: size * 0.085, lineCap: .round))
            Text(value).font(.system(size: size * (value.count > 3 ? 0.25 : 0.34), weight: .semibold))
                .monospacedDigit().rotationEffect(.degrees(90))
        }
        .rotationEffect(.degrees(-90))
        .frame(width: size, height: size)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(
            "\(window?.label ?? "Usage"), \(percent.map { String(format: "%.0f percent used", $0) } ?? (window?.limited == true ? "blocked, percentage unavailable" : "usage unavailable"))\(stale ? ", cached reading" : "")"
        )
    }
}
