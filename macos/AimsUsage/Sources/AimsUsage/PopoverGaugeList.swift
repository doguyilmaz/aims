import SwiftUI
import UsageCore

struct PopoverGaugeList: View {
    let snapshot: UsageSnapshot
    let refreshFailed: Bool

    var body: some View {
        let items = UsageTileItem.items(from: snapshot)
        VStack(spacing: 0) {
            ForEach(Array(items.enumerated()), id: \.element.id) { index, item in
                if index > 0 {
                    let sameProvider = items[index - 1].provider.id == item.provider.id
                    Divider().opacity(sameProvider ? 0.15 : 0.4)
                        .padding(.vertical, sameProvider ? 2 : 6)
                }
                PopoverGaugeRow(provider: item.provider, row: item.row, refreshFailed: refreshFailed)
            }
        }.frame(maxWidth: .infinity)
    }
}
