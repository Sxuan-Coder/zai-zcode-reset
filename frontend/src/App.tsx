import { Navigate, Route, Routes, Outlet } from "react-router-dom";
import { useAuth } from "./auth";
import { Loading } from "./ui";
import Landing from "./pages/Landing";
import Login from "./pages/Login";
import MemberLayout from "./components/MemberLayout";
import Dashboard from "./pages/Dashboard";
import Records from "./pages/Records";
import Usage from "./pages/Usage";
import Settings from "./pages/Settings";
import AdminLayout from "./pages/admin/AdminLayout";
import AdminOverview from "./pages/admin/Overview";
import AdminUsers from "./pages/admin/Users";
import AdminSessions from "./pages/admin/Sessions";
import AdminRules from "./pages/admin/Rules";
import AdminAccounts from "./pages/admin/Accounts";
import AdminLogs from "./pages/admin/Logs";

function RequireAuth({ admin = false }: { admin?: boolean }) {
  const { me, loading } = useAuth();
  if (loading) return <Loading />;
  if (!me) return <Navigate to="/login" replace />;
  if (admin && me.user.role !== "admin") return <Navigate to="/app" replace />;
  return <Outlet />;
}

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<Landing />} />
      <Route path="/login" element={<Login />} />

      <Route element={<RequireAuth />}>
        <Route path="/app" element={<MemberLayout />}>
          <Route index element={<Dashboard />} />
          <Route path="records" element={<Records />} />
          <Route path="usage" element={<Usage />} />
          <Route path="settings" element={<Settings />} />
        </Route>
      </Route>

      <Route element={<RequireAuth admin />}>
        <Route path="/admin" element={<AdminLayout />}>
          <Route index element={<AdminOverview />} />
          <Route path="users" element={<AdminUsers />} />
          <Route path="sessions" element={<AdminSessions />} />
          <Route path="rules" element={<AdminRules />} />
          <Route path="accounts" element={<AdminAccounts />} />
          <Route path="logs" element={<AdminLogs />} />
        </Route>
      </Route>

      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
