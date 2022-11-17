package houston

import "testing"

func TestQueryListGreatestLowerBound(t *testing.T) {
	type args struct {
		v string
	}
	tests := []struct {
		name string
		s    queryList
		args args
		want string
	}{
		{
			name: "current version greater than defined one",
			s:    queryList{{version: "0.29.0", query: "0.29.0 query"}, {version: "0.28.0", query: "0.28.0 query"}, {version: "0.28.2", query: "0.28.2 query"}},
			args: args{v: "0.30.0"},
			want: "0.29.0 query",
		},
		{
			name: "current version equal to smallest defined version",
			s:    queryList{{version: "0.29.0", query: "0.29.0 query"}, {version: "0.28.0", query: "0.28.0 query"}, {version: "0.28.2", query: "0.28.2 query"}},
			args: args{v: "0.28.0"},
			want: "0.28.0 query",
		},
		{
			name: "current version equal to largest defined version",
			s:    queryList{{version: "0.29.0", query: "0.29.0 query"}, {version: "0.28.0", query: "0.28.0 query"}, {version: "0.28.2", query: "0.28.2 query"}},
			args: args{v: "0.29.0"},
			want: "0.29.0 query",
		},
		{
			name: "current version smaller than smallest defined version",
			s:    queryList{{version: "0.29.0", query: "0.29.0 query"}, {version: "0.28.0", query: "0.28.0 query"}, {version: "0.28.2", query: "0.28.2 query"}},
			args: args{v: "0.27.1"},
			want: "0.29.0 query",
		},
		{
			name: "current version in between the range",
			s:    queryList{{version: "0.29.0", query: "0.29.0 query"}, {version: "0.28.0", query: "0.28.0 query"}, {version: "0.28.2", query: "0.28.2 query"}},
			args: args{v: "0.28.3"},
			want: "0.28.2 query",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.GreatestLowerBound(tt.args.v); got != tt.want {
				t.Errorf("%v: queryList.GreatestLowerBound() = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}
