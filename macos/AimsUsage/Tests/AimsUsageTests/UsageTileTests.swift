import Foundation
import Testing
import UsageCore

@testable import AimsUsage

@Test @MainActor func accountsWithTheSameNamesAcrossProvidersKeepSeparateGridCells() throws {
    let snapshot = try UsageSnapshot.decode(
        Data(
            #"""
            {"schemaVersion":1,"generatedAt":"2026-11-12T00:00:00Z","providers":[
              {"id":"claude","name":"Claude","installed":true,"rows":[{"id":"personal","name":"personal"},{"id":"work","name":"work"}]},
              {"id":"codex","name":"Codex","installed":true,"rows":[{"id":"personal","name":"personal"},{"id":"work","name":"work"}]},
              {"id":"kiro","name":"Kiro","installed":true,"rows":[{"id":"credits","name":"credits"}]},
              {"id":"grok","name":"Grok","installed":true,"rows":[{"id":"credits","name":"credits"}]},
              {"id":"agy","name":"Antigravity","installed":true,"rows":[]},
              {"id":"opencode","name":"OpenCode","installed":true,"rows":[{"id":"personal","name":"personal"}]}
            ]}
            """#.utf8))
    let items = UsageTileItem.items(from: snapshot)
    #expect(items.count == 8)
    #expect(Set(items.map(\.id)).count == 8)
    #expect(items.filter { $0.provider.id == "codex" }.compactMap(\.row?.id) == ["personal", "work"])
    #expect(items.contains { $0.provider.id == "grok" && $0.row?.id == "credits" })
    #expect(items.contains { $0.provider.id == "agy" && $0.row == nil })
    #expect(items.last?.provider.id == "opencode")
}
