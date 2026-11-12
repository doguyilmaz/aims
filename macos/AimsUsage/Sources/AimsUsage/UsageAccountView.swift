import SwiftUI
import UsageCore

struct UsageAccountView: View {
    let row: UsageRow
    let tint: Color
    let surface: UsageSurface
    let tile: Bool
    let showDetails: Bool

    var body: some View {
        TimelineView(.periodic(from: .now, by: 60)) { context in
            let window = row.featuredWindow(at: context.date)
            let stale =
                row.isStale(at: context.date) || row.error != nil || window?.needsRefresh(at: context.date) == true
            VStack(alignment: tile ? .center : .leading, spacing: 8) {
                if tile {
                    Text(ProviderStyle.accountName(row))
                        .font(.system(size: 11, weight: .medium)).lineLimit(2).truncationMode(.middle)
                        .frame(height: 28, alignment: .bottom).multilineTextAlignment(.center)
                    UsageGauge(window: window, tint: tint, stale: stale, size: surface == .panel ? 76 : 56)
                    resetCaption(window, now: context.date)
                    if row.isStale(at: context.date) || row.error != nil {
                        Label("Cached", systemImage: "clock").font(.system(size: 10)).foregroundStyle(.secondary)
                    }
                    bankedBadge
                } else {
                    HStack(spacing: 10) {
                        UsageGauge(window: window, tint: tint, stale: stale, size: surface == .panel ? 72 : 52)
                        VStack(alignment: .leading, spacing: 4) {
                            Text(ProviderStyle.accountName(row))
                                .font(.system(size: 12, weight: .medium)).lineLimit(2).truncationMode(.middle)
                            resetCaption(window, now: context.date)
                            bankedBadge
                            if row.error != nil || row.isStale(at: context.date) {
                                Label("Cached", systemImage: "clock")
                                    .font(.system(size: 10)).foregroundStyle(.secondary)
                            }
                        }
                        Spacer(minLength: 0)
                    }
                }
                if showDetails { limitDetails(now: context.date) }
                if let detail = row.detail, !detail.isEmpty {
                    Text(detail).font(.system(size: 11)).foregroundStyle(.secondary).lineLimit(3).help(detail)
                }
            }
            .frame(maxWidth: .infinity, alignment: tile ? .center : .leading)
            .help(observationDescription)
        }
    }

    @ViewBuilder private var bankedBadge: some View {
        if let balance = row.bankedResets {
            Label("\(balance.available) banked", systemImage: "tray.full")
                .font(.system(size: 10, weight: .medium)).foregroundStyle(tint)
        }
    }

    private var observationDescription: String {
        let age =
            row.observedAt.map { "Reported \($0.formatted(date: .abbreviated, time: .shortened))" }
            ?? "No observation reported"
        return [row.name, row.plan, age, row.error].compactMap { $0 }.joined(separator: " · ")
    }

    @ViewBuilder private func resetCaption(_ window: UsageWindow?, now: Date) -> some View {
        if let window {
            VStack(alignment: tile ? .center : .leading, spacing: 2) {
                Text(window.label == "month" ? "Monthly" : window.label == "budget" ? "Budget" : window.label)
                    .font(.system(size: 10, weight: .medium)).foregroundStyle(.secondary)
                if let date = window.resetsAt {
                    Text(
                        window.needsRefresh(at: now)
                            ? "Refresh needed" : ResetPresentation.countdown(to: date, now: now)
                    )
                    .font(.system(size: 11)).monospacedDigit()
                    .foregroundStyle(window.needsRefresh(at: now) ? .orange : .secondary)
                    .help("Resets \(date.formatted(date: .complete, time: .complete))")
                } else {
                    Text("Reset unknown").font(.system(size: 11)).foregroundStyle(.secondary)
                }
            }
        } else {
            Text("No reading").font(.system(size: 11)).foregroundStyle(.secondary)
        }
    }

    private func limitDetails(now: Date) -> some View {
        VStack(spacing: 6) {
            ForEach(Array((row.windows ?? []).enumerated()), id: \.offset) { _, window in
                VStack(spacing: 4) {
                    HStack {
                        Text(window.label == "month" ? "Monthly" : window.label)
                        Spacer(minLength: 4)
                        Text(
                            window.validPercent.map { String(format: "%.0f%%", $0) }
                                ?? (window.limited == true ? "Blocked" : "Unknown")
                        ).monospacedDigit()
                    }.font(.system(size: 11))
                    GeometryReader { geometry in
                        Capsule().fill(tint.opacity(0.10))
                            .overlay(alignment: .leading) {
                                Capsule().fill(window.needsRefresh(at: now) ? Color.secondary : tint)
                                    .frame(
                                        width: geometry.size.width * min(1, max(0, (window.validPercent ?? 0) / 100)))
                            }
                    }.frame(height: 3).accessibilityHidden(true)
                    if let date = window.resetsAt {
                        Text(
                            window.needsRefresh(at: now)
                                ? "Refresh needed" : ResetPresentation.countdown(to: date, now: now)
                        )
                        .font(.system(size: 10)).foregroundStyle(.secondary)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .help(date.formatted(date: .complete, time: .complete))
                    }
                }
            }
        }
    }
}
