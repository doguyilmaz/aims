import Testing

@testable import AimsUsage

@Test @MainActor func gaugesUseTheSameNarrowLayoutInTheMenuAndPinnedPanel() {
    #expect(UsageDisplayStyle.gauges.menuWidth == UsageDisplayStyle.gauges.preferredWidth)
    #expect(UsageDisplayStyle.gauges.preferredWidth <= UsageDisplayStyle.compact.preferredWidth)
}
