import Testing

@testable import AimsUsage

@Test @MainActor func gaugesKeepTheSharedOverviewNarrow() {
    #expect(UsageDisplayStyle.gauges.preferredWidth == 340)
    #expect(UsageDisplayStyle.gauges.preferredWidth <= UsageDisplayStyle.compact.preferredWidth)
}
