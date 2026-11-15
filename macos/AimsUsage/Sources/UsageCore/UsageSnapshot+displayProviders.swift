extension UsageSnapshot {
    public var displayProviders: [UsageProvider] {
        let rank = ["claude": 0, "codex": 1, "kiro": 2, "grok": 3, "agy": 4, "opencode": Int.max]
        return providers.enumerated().sorted { left, right in
            let lhs = rank[left.element.id] ?? 5
            let rhs = rank[right.element.id] ?? 5
            return lhs == rhs ? left.offset < right.offset : lhs < rhs
        }.map(\.element)
    }
}
