import SwiftUI

struct ReminderListView: View {
    @Bindable var model: UsageModel

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                Text("Your reminders").font(.system(size: 14, weight: .semibold))
                ForEach(model.snapshot?.customResets ?? []) { reminder in
                    HStack(alignment: .top) {
                        VStack(alignment: .leading, spacing: 4) {
                            Text(reminder.label).font(.system(size: 12, weight: .medium))
                            Text(
                                "\(reminder.provider) / \(reminder.row) · \(reminder.at.formatted(date: .abbreviated, time: .shortened))"
                            )
                            .font(.system(size: 11)).foregroundStyle(.secondary)
                        }
                        Spacer(minLength: 0)
                        UsageIconButton(
                            title: "Remove reminder \(reminder.label)", icon: .trash, tint: .red, role: .destructive
                        ) {
                            Task { await model.removeReminder(id: reminder.id) }
                        }
                        .disabled(model.isRefreshing || model.fixtureURL != nil)
                    }
                }
                Text("Personal reminders do not change provider quota.").font(.system(size: 11)).foregroundStyle(
                    .secondary)
            }.padding(16)
        }.frame(maxHeight: 360)
            .scrollIndicators(.automatic).background(UsageScrollStyle())
            .environment(\.usageSlimScrollbars, model.slimScrollbars)
    }
}
