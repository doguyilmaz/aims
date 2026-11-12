import SwiftUI
import UsageCore

struct UsageGauge: View {
    let window: UsageWindow?
    let tint: Color
    let stale: Bool
    let size: CGFloat

    private var percent: Double? { window?.validPercent }
    private var accent: Color {
        if stale { return .secondary }
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
            Circle().trim(from: 0, to: 0.75)
                .stroke(accent.opacity(0.13), style: StrokeStyle(lineWidth: size * 0.085, lineCap: .round))
            Circle().trim(from: 0, to: 0.75 * min(1, max(0, (percent ?? 0) / 100)))
                .stroke(
                    AngularGradient(colors: [accent.opacity(0.5), accent], center: .center),
                    style: StrokeStyle(lineWidth: size * 0.085, lineCap: .round))
            VStack(spacing: 0) {
                HStack(alignment: .firstTextBaseline, spacing: 0) {
                    Text(value).font(.system(size: size * 0.30, weight: .semibold, design: .rounded))
                    if percent != nil { Text("%").font(.system(size: size * 0.15, weight: .medium)) }
                }.monospacedDigit()
                Text(percent != nil ? "used" : window?.limited == true ? "blocked" : "unknown")
                    .font(.system(size: max(8, size * 0.14))).foregroundStyle(.secondary)
            }
            .rotationEffect(.degrees(-135))
            .offset(x: -size * 0.015, y: size * 0.015)
        }
        .rotationEffect(.degrees(135))
        .frame(width: size, height: size)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(
            "\(window?.label ?? "Usage"), \(percent.map { String(format: "%.0f percent used", $0) } ?? (window?.limited == true ? "blocked, percentage unavailable" : "usage unavailable"))\(stale ? ", cached reading" : "")"
        )
    }
}
