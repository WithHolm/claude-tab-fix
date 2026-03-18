interface User {
    id: string;
    name: string;
    email: string;
    role: "admin" | "user" | "guest";
}

interface ApiResponse<T> {
    data: T;
    error: string | null;
    status: number;
}

async function fetchUser(id: string): Promise<ApiResponse<User>> {
    try {
        const response = await fetch(`/api/users/${id}`);
        if (!response.ok) {
            return {
                data: null as unknown as User,
                error: `HTTP ${response.status}`,
                status: response.status,
            };
        }
        const data = await response.json();
        return { data, error: null, status: 200 };
    } catch (err) {
        return {
            data: null as unknown as User,
            error: err instanceof Error ? err.message : "unknown error",
            status: 0,
        };
    }
}

function formatUser(user: User): string {
    if (user.role === "admin") {
        return `[ADMIN] ${user.name} <${user.email}>`;
    }
    return `${user.name} <${user.email}>`;
}

export class UserService {
    private cache: Map<string, User> = new Map();

    async getUser(id: string): Promise<User | null> {
        if (this.cache.has(id)) {
            return this.cache.get(id)!;
        }

        const result = await fetchUser(id);
        if (result.error !== null) {
            console.error(`failed to fetch user ${id}: ${result.error}`);
            return null;
        }

        this.cache.set(id, result.data);
        return result.data;
    }

    async getUsers(ids: string[]): Promise<User[]> {
        const results = await Promise.all(ids.map((id) => this.getUser(id)));
        return results.filter((u): u is User => u !== null);
    }

    formatAll(users: User[]): string[] {
        return users.map(formatUser);
    }
}
