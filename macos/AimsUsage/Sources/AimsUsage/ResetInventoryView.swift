import SwiftUI
import UsageCore

struct ResetInventoryView: View {
    let balance: ResetBalance?

    var body: some View {
        TimelineView(.periodic(from: .now, by: 60)) { context in
            VStack(alignment: .leading, spacing: 8) {
                if let balance {
                    Text("\(balance.available) banked reset\(balance.available == 1 ? "" : "s") available").font(
                        .system(size: 12, weight: .medium))
                    ForEach(Array((balance.credits ?? []).enumerated()), id: \.offset) { _, credit in
                        VStack(alignment: .leading, spacing: 3) {
                            Text(credit.title ?? "Banked reset").font(.system(size: 11, weight: .medium))
                            if credit.status != "available" {
                                Text(credit.status).font(.system(size: 10)).foregroundStyle(.secondary)
                            }
                            if let date = credit.expiresAt {
                                Text(
                                    date <= context.date
                                        ? "Expired · refresh to check"
                                        : "Expires \(ResetPresentation.countdown(to: date, now: context.date))"
                                )
                                .font(.system(size: 11)).foregroundStyle(.secondary)
                                .help(date.formatted(date: .complete, time: .complete))
                            } else {
                                Text(credit.noExpiry == true ? "Does not expire" : "Expiry not reported")
                                    .font(.system(size: 11)).foregroundStyle(.secondary)
                            }
                        }
                    }
                    if balance.detailsAreIncomplete {
                        Text(
                            balance.credits == nil
                                ? "Expiry details not reported"
                                : "Details for \(balance.detailCount) of \(balance.available); other expiries unknown"
                        )
                        .font(.system(size: 10)).foregroundStyle(.secondary)
                    }
                } else {
                    Text("Banked reset inventory not reported").font(.system(size: 11)).foregroundStyle(.secondary)
                }
            }.frame(maxWidth: .infinity, alignment: .leading)
        }
    }
}
