"use client";

import { RequireAuth } from "@/components/require-auth";
import { Container } from "@/components/ui";
import { NotificationList } from "./notification-list";

export default function NotificationsPage() {
  return (
    <Container className="max-w-2xl py-8">
      <RequireAuth>{(user) => <NotificationList role={user.role} />}</RequireAuth>
    </Container>
  );
}
