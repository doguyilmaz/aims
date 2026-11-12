import SwiftUI
import UsageCore

struct UsageAccountView: View {
    let row: UsageRow
    let showBanked: Bool
    let tint: Color

    var body: some View {
        TimelineView(.periodic(from: .now, by: 60)) { context in
            VStack(alignment: .leading, spacing: 10) {
                HStack(alignment: .firstTextBaseline) {
                    Text(row.name).font(.system(size: 12, weight: .medium)).lineLimit(2)
                    Spacer(minLength: 6)
                    if let plan = row.plan {
                        Text(plan.replacingOccurrences(of: "_", with: " ")).font(.system(size: 9)).foregroundStyle(
                            .secondary
                        ).lineLimit(1)
                    }
                }
                if (row.windows ?? []).isEmpty {
                    Text("Usage not reported").font(.caption2).foregroundStyle(.secondary)
                }
                if let error = row.error {
                    Text("Last check: \(error) · previous reading retained").font(.caption2).foregroundStyle(.orange)
                }
                ForEach(Array((row.windows ?? []).enumerated()), id: \.offset) { _, window in
                    windowView(window, now: context.date)
                }
                if let detail = row.detail { Text(detail).font(.caption2).foregroundStyle(.secondary) }
                if showBanked { bankedView(now: context.date) }
                HStack(spacing: 4) {
                    Image(systemName: row.isStale(at: context.date) ? "clock.badge.exclamationmark" : "clock")
                    if let at = row.observedAt {
                        Text("Reported \(at.formatted(.relative(presentation: .numeric)))")
                    } else {
                        Text("Not checked")
                    }
                    if row.isStale(at: context.date) { Text("· stale") }
                }.font(.system(size: 9)).foregroundStyle(row.isStale(at: context.date) ? .orange : .secondary)
            }
        }
    }

    private func windowView(_ window: UsageWindow, now: Date) -> some View {
        let expired = window.needsRefresh(at: now)
        let percent = window.validPercent
        return VStack(alignment: .leading, spacing: 5) {
            HStack {
                Text(ResetPresentation.period(window.label)).foregroundStyle(.secondary)
                Spacer()
                Text(
                    percent.map { String(format: "%.0f%% used", $0) }
                        ?? (window.limited == true ? "Blocked" : "Unknown")
                )
                .fontWeight(.semibold).monospacedDigit().foregroundStyle(expired ? .secondary : .primary)
            }.font(.system(size: 10))
            GeometryReader { geometry in
                Capsule().fill(.white.opacity(0.08))
                    .overlay(alignment: .leading) {
                        Capsule().fill(expired ? Color.gray : (percent ?? 0 >= 90 ? .orange : tint))
                            .frame(width: geometry.size.width * min(1, max(0, (percent ?? 0) / 100)))
                    }
            }.frame(height: 4).accessibilityLabel(
                "\(window.label), \(percent.map { String(format: "%.0f percent used", $0) } ?? "unknown usage")")
            HStack(alignment: .firstTextBaseline) {
                if let reset = window.resetsAt {
                    Text(
                        expired
                            ? "Reset passed · refresh needed"
                            : "Resets \(ResetPresentation.countdown(to: reset, now: now))")
                    Spacer(minLength: 4)
                    Text(reset, format: .dateTime.weekday(.abbreviated).hour().minute())
                        .help(reset.formatted(date: .complete, time: .complete))
                } else {
                    Text("Reset time not reported")
                }
            }.font(.system(size: 9)).foregroundStyle(expired ? .orange : .secondary)
        }
    }

    @ViewBuilder private func bankedView(now: Date) -> some View {
        if let balance = row.bankedResets {
            HStack(spacing: 5) {
                Image(systemName: "tray.full")
                Text("\(balance.available) reported banked reset\(balance.available == 1 ? "" : "s")").fontWeight(
                    .medium)
            }.font(.system(size: 10)).foregroundStyle(tint)
            ForEach(balance.credits ?? []) { credit in
                HStack(alignment: .firstTextBaseline) {
                    Text(credit.title ?? "Banked reset").lineLimit(2)
                    if credit.status != "available" { Text("(\(credit.status))") }
                    Spacer()
                    if let expiry = credit.expiresAt {
                        Text(
                            expiry <= now
                                ? "Expired · refresh" : "Expires \(ResetPresentation.countdown(to: expiry, now: now))"
                        )
                        .help(expiry.formatted(date: .complete, time: .complete))
                    } else {
                        Text("No expiry reported")
                    }
                }.font(.system(size: 9)).foregroundStyle(.secondary)
            }
            if balance.credits == nil && balance.available > 0 {
                Text("Expiry details not reported").font(.system(size: 9)).foregroundStyle(.tertiary)
            }
        } else {
            Text("Banked resets · not reported").font(.system(size: 9)).foregroundStyle(.tertiary)
        }
    }
}
