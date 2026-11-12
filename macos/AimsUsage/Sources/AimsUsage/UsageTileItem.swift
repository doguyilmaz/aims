import UsageCore

struct UsageTileItem: Identifiable {
    struct ID: Hashable {
        let provider: String
        let row: String?
    }

    let provider: UsageProvider
    let row: UsageRow?
    var id: ID { ID(provider: provider.id, row: row?.id) }

    static func items(from snapshot: UsageSnapshot) -> [UsageTileItem] {
        snapshot.displayProviders.flatMap { provider in
            provider.rows.isEmpty
                ? [UsageTileItem(provider: provider, row: nil)]
                : provider.rows.map { UsageTileItem(provider: provider, row: $0) }
        }
    }
}
